package main

// https://apereo.github.io/cas/5.0.x/protocol/CAS-Protocol-Specification.html
// https://github.com/jmcarbo/golacas
// https://github.com/apognu/gocas

import (
	"flag"
	"fmt"
	"github.com/gin-contrib/location"
	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
	"github.com/robfig/cron"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"

	"github.com/itsjamie/gin-cors"

	"github.com/ulule/limiter"
	mgin "github.com/ulule/limiter/drivers/middleware/gin"
	"github.com/ulule/limiter/drivers/store/memory"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())
var hashKey = []byte(config.HashSecret)
var blockKey = []byte(config.Secret)
var secure = securecookie.New(hashKey, blockKey)

func RandString(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

type Ticket struct {
	Class     string
	Value     string
	User      string
	Service   string
	CreatedAt time.Time
	Renew     bool
}

func NewTicket(class string, service string, user string, renew bool) *Ticket {
	t := Ticket{
		Class:     class,
		Value:     class + "-" + RandString(32),
		CreatedAt: time.Now(),
		User:      user,
		Service:   service,
		Renew:     renew,
	}
	mutex.Lock()
	tickets[t.Value] = t
	mutex.Unlock()
	return &t
}

func GetTicket(value string) *Ticket {
	mutex.Lock()
	t, ok := tickets[value]
	mutex.Unlock()
	if ok {
		return &t
	}
	return nil
}

func DeleteTicket(value string) {
	mutex.Lock()
	delete(tickets, value)
	mutex.Unlock()
}

func NewTGC(ctx *gin.Context, ticket *Ticket) {
	cookie := &http.Cookie{Name: cookieName, Path: *basePath}
	tgt := NewTicket("TGT", ticket.Service, ticket.User, false)
	encodedValue, _ := secure.Encode(cookieName, tgt.Value)

	log.Debug(fmt.Sprintf("New TGC User: <%s>", ticket.User))
	cookie.Value = encodedValue
	http.SetCookie(ctx.Writer, cookie)
}

func GetTGC(ctx *gin.Context) *Ticket {
	payload, _ := ctx.Cookie(cookieName)
	var decodedValue string
	secure.Decode(cookieName, payload, &decodedValue)
	return GetTicket(decodedValue)
}

func DeleteTGC(ctx *gin.Context) {
	ticket := GetTGC(ctx)
	if ticket != nil {
		DeleteTicket(ticket.Value)
	}

	cookie := &http.Cookie{Name: cookieName, Path: *basePath}
	cookie.Value = "deleted"
	http.SetCookie(ctx.Writer, cookie)

	return
}

var (
	basePath   = flag.String("basepath", "", "basepath")
	backend    = flag.String("backend", "test", "user validate : [test|ldap]")
	tickets    = map[string]Ticket{}
	cookieName = "CASTGC"
	mutex      = &sync.Mutex{}
	port       = flag.String("port", "3004", "CAS listening port")
	debug      = flag.Bool("debug", false, "Debug, doesn't log to file")
	conf       = flag.String("conf", "", "Optional INI config file")
	config     = Config{
		Port:       ":3004",
		Secret:     "0123456789123456",
		HashSecret: "very-secret",
		LdapServer: "ldap.example.org",
		LdapBind:   "ou=people,dc=example,dc=org",
	}
	garbageCollectionPeriod = 5
)

func loadTemplates(list ...string) multitemplate.Render {
	r := multitemplate.New()

	for _, x := range list {
		templateString, err := Asset("tmpl/" + x)
		if err != nil {
			fmt.Println(err)
			os.Exit(0)
		}

		tmplMessage, err := template.New(x).Parse(string(templateString))
		if err != nil {
			fmt.Println(err)
			os.Exit(0)
		}

		r.Add(x, tmplMessage)
	}

	return r
}

func init() {
	//if flag.Lookup("test.v") != nil {
	if !strings.HasSuffix(os.Args[0], ".test") {
		flag.Parse()
	} else {
		*debug = true
	}

	config, _ = readConf(config, *conf)
	if *debug {
		fmt.Printf("%+v\n", config)
	}

	confLog(config.LogPath)

}

func main() {
	cr := cron.New()
	cr.AddFunc(fmt.Sprintf("@every %dm", garbageCollectionPeriod), collectTickets)
	cr.Start()

	setupServer().Run(":" + *port)

	cr.Stop()
}

// The engine with all endpoints is now extracted from the main function
func setupServer() *gin.Engine {

	rate, err := limiter.NewRateFromFormatted("10-M") // 10 reqs/minute
	if err != nil {
		panic(err)
	}
	lStore := memory.NewStore()
	middleware := mgin.NewMiddleware(limiter.New(lStore, rate))

	r := gin.New() //Default()
	if *debug == false {
		r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
			// custom format
			return fmt.Sprintf("%s - [%s] \"%s %s %s\" %d \"%s\" %s\n",
				param.ClientIP,
				param.TimeStamp.Format(time.RFC3339),
				param.Method,
				param.Path,
				param.Request.Proto,
				param.StatusCode,
				param.Request.UserAgent(),
				param.ErrorMessage,
			)
		}))
	} else {
		r.Use(gin.Logger())
	}
	r.Use(gin.Recovery())
	r.ForwardedByClientIP = true
	r.Use(middleware)

	/*  ========================================= */
	r.Use(cors.Middleware(cors.Config{
		Origins:         "*",
		Methods:         "GET, POST",
		RequestHeaders:  "Origin, Content-Type",
		MaxAge:          50 * time.Second,
		ValidateHeaders: false,
	}))

	store := cookie.NewStore(blockKey)
	store.Options(sessions.Options{
		//Domain:   "localhost",
		SameSite: http.SameSiteStrictMode,
	})
	//store := memstore.NewStore([]byte("config.Secret"))
	r.Use(sessions.Sessions("mysession", store))
	/*  ========================================= */
	r.Use(location.Default())

	//r.LoadHTMLGlob("tmpl/*")
	r.HTMLRender = loadTemplates("login.tmpl")
	setApi(r)

	log.Info("Server started")

	//r.Run(":" + *port)

	return r
}

func collectTickets() {
	//fmt.Printf("Cleaning tickets\n")
	numTicketsCollected := 0
	m5, _ := time.ParseDuration(fmt.Sprintf("%dm", garbageCollectionPeriod))
	five := time.Now().Add(-m5)
	mutex.Lock()
	for k, v := range tickets {
		if (v.Class == "ST") && v.CreatedAt.Before(five) {
			delete(tickets, k)
			numTicketsCollected++
		}
	}
	mutex.Unlock()
	if numTicketsCollected > 0 {
		log.Info(fmt.Sprintf("%d tickets cleaned", numTicketsCollected))
		//fmt.Printf(" Tickets : %+v\n", tickets)
	}
}

func setApi(r *gin.Engine) {
	r.GET("/login", login)
	r.POST("/login", loginPost)
	r.GET("/logout", logout)
	r.GET("/validate", validate)               // CASv1
	r.GET("/serviceValidate", serviceValidate) // CASv2
	// r.GET("/p3/serviceValidate", serviceValidateV3) // CASv3
}

func parseService(service string) (string, url.URL, url.Values) {
	if service == "" {
		return "", url.URL{}, nil
	}
	decodedValue, _ := url.QueryUnescape(service)
	u, _ := url.Parse(decodedValue)
	m, _ := url.ParseQuery(u.RawQuery)

	s := fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, u.Path)
	q := url.Values{}
	for v, e := range m {
		if v != "ticket" {
			q.Set(v, strings.Join(e, "\n"))
		}
	}
	//query := q.Encode()
	//query := strings.Replace(q.Encode(), "+", "%20", -1)
	//query = strings.Replace(query, "%21", "!", -1)
	location := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	return s, location, q
}

func getLocalURL(c *gin.Context) string {
	url := location.Get(c)
	return url.String()
}

func login(c *gin.Context) {
	service := c.Query("service")
	log.Debug(c.ClientIP(), " - GET /login")
	tgc := GetTGC(c)
	if tgc != nil {
		log.Info(c.ClientIP(), " - TGC for: ", tgc.User)
		localservice := getLocalURL(c) + "/login"
		serv, l, q := parseService(service)
		st := NewTicket("ST", serv, tgc.User, false)
		if serv != "" && serv != localservice {
			log.Debug("new service: ", serv)
			q.Set("ticket", st.Value)
			l.RawQuery = q.Encode()
			log.Debug("Redirect to Service: " + l.String())
			c.Redirect(302, l.String())
			return
		}
		//service = localservice + "?ticket=" + st.Value
		//fmt.Println("new service 2: ", service)
	}

	log.Debug("no TGC")
	lt := NewTicket("LT", "", "", false)
	c.HTML(http.StatusOK, "login.tmpl", gin.H{
		"title": "CAS Login",
		"lt":    lt.Value,
	})
}

func testValidateUser(username, password string) bool {
	log.Debug(fmt.Sprintf("Validate test User <%s> <%s>", username, password))
	if username == "" {
		return false
	}
	if username != password {
		return false
	}
	return true
}

func loginPost(c *gin.Context) {
	log.Debug(c.ClientIP(), " - POST /login")
	session := sessions.Default(c)
	var s Status
	t := session.Get("status")
	if t != nil {
		s = StrToStatus(t.(string))
	}
	//fmt.Printf("=> count %+v\n", s.Count)
	s = userFailLimiter(s, 30)

	service := c.Query("service")
	username := c.PostForm("username")
	password := c.PostForm("password")

	var IsGoodChar = regexp.MustCompile(`^[a-zA-Z0-9\.\@]+$`).MatchString
	if IsGoodChar(username) == false {
		log.Error("Bad Char in username")
		username = ""
	}
	if len(username) > 64 {
		log.Error("username too long")
		username = ""
	}
	if len(password) > 256 {
		log.Error("password too long")
		password = ""
	}

	//lt := c.PostForm("lt") // TODO validate lt

	switch {
	case s.Lock == true:
		session.Set("status", s.ToJSONStr())
		session.Save()
		c.Header("Content-Type", "text/html")
		c.String(200, "<html>Too many errors, come back later</html>")
		log.Debug(c.ClientIP(), " - Lock Status")
	case username != "" && password != "":
		valid := false
		if *backend == "test" {
			valid = testValidateUser(username, password)
		}
		if *backend == "ldap" {
			valid = ldapValidateUser(username, password, config)
		}
		if valid == true {
			s.User = username
			s.Count = 0
			s.Confirm = false
			session.Set("status", s.ToJSONStr())
			session.Save()

			log.Info(c.ClientIP(), " - valid AUTHENTICATION [username:", username, "]")

			serv, l, q := parseService(service)
			st := NewTicket("ST", serv, username, true)
			NewTGC(c, st)
			if service != "" {
				q.Set("ticket", st.Value)
				l.RawQuery = q.Encode()
				log.Debug("Post Redirect to Service: " + l.String())
				c.Redirect(302, l.String())
			} else {
				log.Info("Auth without service")
				c.Redirect(303, getLocalURL(c)+"/login")
			}
		} else {
			log.Info(c.ClientIP(), " - AUTHENTICATION failed for ", username)
			session.Set("status", s.ToJSONStr())
			session.Save()
			c.Header("Content-Type", "text/html")
			c.String(200, "<html>bad user or pass</html>")
		}
	default:
		log.Error(c.ClientIP(), " - Bad Post params")
		c.Header("Content-Type", "text/html")
		c.String(200, "<html>Error</html>")
		//c.Redirect(303, getLocalURL(c) + "/login" )
	}
}

func logout(c *gin.Context) {
	log.Debug("Logout")
	tgc := GetTGC(c)
	if tgc != nil {
		log.Info(c.ClientIP(), " - Logout: DeleteTGC for ", tgc.User)
		DeleteTGC(c)
		c.Writer.Write([]byte("User has been logged out"))
	} else {
		c.Writer.Write([]byte("User is not logged in"))
	}
}

func serviceValidate(c *gin.Context) {
	service := c.Query("service")
	ticket := c.Query("ticket")
	serv, _, _ := parseService(service)

	log.Debug(fmt.Sprintf("CASv2: serviceValidate <%s> <%s>", service, ticket))
	if ticket == "" {
		log.Debug("INVALID_TICKET, empty ticket")
		c.Writer.Write(NewCASFailureResponse("INVALID_TICKET", "Empty Ticket"))
	} else {
		t := GetTicket(ticket)
		if t == nil {
			log.Debug("INVALID_TICKET, Ticket not recognized")
			c.Writer.Write(NewCASFailureResponse("INVALID_TICKET", "Ticket not recognized"))
		} else {
			if t.Service != serv {
				log.Debug("INVALID_SERVICE")
				c.Writer.Write(NewCASFailureResponse("INVALID_SERVICE", "Ticket was used for another service than it was generated for"))
			} else {
				DeleteTicket(ticket)
				//fmt.Printf("=> User <%s>\n", t.User)
				log.Info(c.ClientIP(), " - AUTHENTICATION_CASv2 [username:", t.User, "]")
				c.Writer.Write(NewCASSuccessResponse(t.User))
			}
		}
	}
}

func validate(c *gin.Context) {
	service := c.Query("service")
	ticket := c.Query("ticket")

	log.Debug(fmt.Sprintf("CASv1: validate <%s> <%s>\n", service, ticket))
	if ticket == "" {
		c.Writer.Write([]byte("no\n"))
	} else {
		t := GetTicket(ticket)
		if t == nil {
			c.Writer.Write([]byte("no\n"))
		} else {
			if t.Service != service {
				c.Writer.Write([]byte("no\n"))
			} else {
				DeleteTicket(ticket)
				//fmt.Printf("=> User <%s>\n", t.User)
				log.Info(c.ClientIP(), " - AUTHENTICATION_CASv1 [username:", t.User, "]")
				c.Writer.Write([]byte("yes\n" + t.User + "\n"))
			}
		}
	}
}
