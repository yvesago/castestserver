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
	"os"
	"strings"
	"sync"
	"time"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())
var hashKey = []byte("very-secret")
var blockKey = []byte("0123456789123456")
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
	} else {
		return nil
	}
}

func DeleteTicket(value string) {
	mutex.Lock()
	delete(tickets, value)
	mutex.Unlock()
}

func NewTGC(ctx *gin.Context, ticket *Ticket) {
	cookie := &http.Cookie{Name: cookieName, Path: *basePath}
	tgt := NewTicket("TGT", ticket.Service, ticket.User, false)
	encoded_value, _ := secure.Encode(cookieName, tgt.Value)

	fmt.Printf("New TGC User: <%s>\n", ticket.User)
	cookie.Value = encoded_value
	http.SetCookie(ctx.Writer, cookie)
}

func GetTGC(ctx *gin.Context) *Ticket {
	payload, _ := ctx.Cookie(cookieName)
	var decoded_value string
	secure.Decode(cookieName, payload, &decoded_value)
	return GetTicket(decoded_value)
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
	basePath                = flag.String("basepath", "", "basepath")
	tickets                 = map[string]Ticket{}
	cookieName              = "CASTGC"
	mutex                   = &sync.Mutex{}
	port                    = flag.String("port", "3004", "CAS listening port")
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

func main() {
	cr := cron.New()
	cr.AddFunc(fmt.Sprintf("@every %dm", garbageCollectionPeriod), collectTickets)
	cr.Start()

	flag.Parse()

	r := gin.Default()
	r.Use(location.Default())

	//r.LoadHTMLGlob("tmpl/*")
	r.HTMLRender = loadTemplates("login.tmpl")
	setApi(r)
	r.Run(":" + *port)

	cr.Stop()
}

func collectTickets() {
	fmt.Printf("Cleaning tickets\n")
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
	fmt.Printf("%d tickets cleaned\n", numTicketsCollected)
	//fmt.Printf(" Tickets : %+v\n", tickets)
}

func setApi(r *gin.Engine) {
	r.GET("/login", login)
	r.POST("/login", loginPost)
	r.GET("/logout", logout)
	r.GET("/validate", validate)               // CASv1
	r.GET("/serviceValidate", serviceValidate) // CASv2
	// r.GET("/p3/serviceValidate", serviceValidateV3) // CASv3
}

func getLocalURL(c *gin.Context) string {
	url := location.Get(c)
	return url.String()
}

func login(c *gin.Context) {
	service := c.Query("service")
	tgc := GetTGC(c)
	if tgc != nil {
		fmt.Println("login: with TGC, user:", tgc.User)
		localservice := getLocalURL(c) + "/login"
		st := NewTicket("ST", service, tgc.User, false)
		if service != "" && service != localservice {
			p := "?"
			if strings.Contains(service, "?") {
				p = "&"
			}
			service = service + p + "ticket=" + st.Value
			c.Redirect(303, service)
			return
		}
	}

	lt := NewTicket("LT", "", "", false)
	c.HTML(http.StatusOK, "login.tmpl", gin.H{
		"title": "CAS Login",
		"lt":    lt.Value,
	})
}

func validateUser(username, password string) bool {
	fmt.Printf("- validateUser <%s> <%s>\n", username, password)

	if username == "" {
		return false
	}

	if username != password {
		return false
	}

	/*	if *domain != "" && !strings.Contains(username, *domain) {
				username = username + "@" + *domain
			}

		    c, err := ldap.Dial(*ldapServer)
			if err != nil {
				fmt.Println(err)
				return false
			}
			err = c.Bind(username, password)
			if err != nil {
				fmt.Println(err)
				return false
			}*/
	return true
}

func loginPost(c *gin.Context) {
	service := c.Query("service")
	username := c.PostForm("username")
	password := c.PostForm("password")
	//lt := c.PostForm("lt") // TODO validate lt

	if service == "" {
		fmt.Println("pb: Missing service")
		service = getLocalURL(c) + "/login"
	}
	if !validateUser(username, password) {
		fmt.Println("pb: Validateuser false")
		service = getLocalURL(c) + "/login"
	} else {
		fmt.Println("ok: Validateuser true")
		st := NewTicket("ST", service, username, true)
		NewTGC(c, st)
		p := "?"
		if strings.Contains(service, "?") {
			p = "&"
		}
		service = service + p + "ticket=" + st.Value //TODO manage ? or &
	}
	fmt.Println("Redirect to Service: " + service)
	c.Redirect(303, service)
}

func logout(c *gin.Context) {
	fmt.Printf("Logout\n")
	tgc := GetTGC(c)
	if tgc != nil {
		fmt.Printf("Logout: DeleteTGC\n")
		DeleteTGC(c)
		c.Writer.Write([]byte("User has been logged out"))
	} else {
		c.Writer.Write([]byte("User is not logged in"))
	}
}

func serviceValidate(c *gin.Context) {
	service := c.Query("service")
	ticket := c.Query("ticket")

	fmt.Printf("CASv2: serviceValidate <%s> <%s>\n", service, ticket)
	if ticket == "" {
		c.Writer.Write(NewCASFailureResponse("INVALID_TICKET", "Ticket not recognized"))
	} else {
		t := GetTicket(ticket)
		if t == nil {
			c.Writer.Write(NewCASFailureResponse("INVALID_TICKET", "Ticket not recognized"))
		} else {
			if t.Service != service {
				c.Writer.Write(NewCASFailureResponse("INVALID_SERVICE", "Ticket was used for another service than it was generated for"))
			} else {
				DeleteTicket(ticket)
				fmt.Printf("=> User <%s>\n", t.User)
				c.Writer.Write(NewCASSuccessResponse(t.User))
			}
		}
	}
}

func validate(c *gin.Context) {
	service := c.Query("service")
	ticket := c.Query("ticket")

	fmt.Printf("CASv1: validate <%s> <%s>\n", service, ticket)
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
				fmt.Printf("=> User <%s>\n", t.User)
				c.Writer.Write([]byte("yes\n" + t.User + "\n"))
			}
		}
	}
}
