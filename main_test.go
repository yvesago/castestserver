package main

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

var srv *httptest.Server

func TestMain(t *testing.T) {
	// The setupServer method is injected into a test server
	authSrv := httptest.NewServer(setupServer())
	// Shut down the server and block until all requests have gone through
	defer authSrv.Close()
	u, _ := url.Parse(authSrv.URL)

	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
	}

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//fmt.Printf("Service server:\n => %#v\n => %s\n", r, r.RequestURI)
		for _, cookie := range r.Cookies() {
			fmt.Println("Validate set a cookie named:", cookie.Name)
		}
		ru := strings.Replace(r.RequestURI,"/?","",1)
		m, _ := url.ParseQuery(ru)
		ticket := m["ticket"][0]
		fmt.Printf("====>%s<==\n", ticket)
		data := url.Values{}
		data.Set("ticket", ticket)
		data.Set("service", srv.URL+"/")

		respV, err := http.Get(fmt.Sprintf("%s/serviceValidate/?%s", authSrv.URL, data.Encode()))

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if respV.StatusCode != 200 {
			t.Fatalf("Expected status code 200, got %v", respV.StatusCode)
		}
		w.WriteHeader(200)
		w.Write([]byte("Success!"))
	}))
	defer srv.Close()

	fmt.Printf("Auth server: %s\n", authSrv.URL)
	fmt.Printf("Service server: %s\n\n", srv.URL)

	fmt.Println("===============\n  Get /login")
	// Make a request to our server with the {base url}/login
	resp, err := client.Get(fmt.Sprintf("%s/login", authSrv.URL))

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Expected status code 200, got %v", resp.StatusCode)
	}

	val, ok := resp.Header["Content-Type"]
	fmt.Println(val)

	// Assert that the "content-type" header is actually set
	if !ok {
		t.Fatalf("Expected Content-Type header to be set")
	}
    // no cookies on GET
	for _, cookie := range jar.Cookies(u) {
		fmt.Printf(" Client with a cookie named: %s\n", cookie.Name)
	}


	// Login with user/user
	fmt.Println("===============\n  Login with user/user")
	formData := url.Values{}
	formData.Set("username", "user")
	formData.Set("password", "user")
	service := srv.URL + "/?truc=truc&test=test"

	req, errP := http.NewRequest("POST", fmt.Sprintf("%s/login/?service=%s", authSrv.URL, url.QueryEscape(service)), strings.NewReader(formData.Encode()))

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	//fmt.Printf("%#v\n",req)
	if errP != nil {
		t.Fatalf("Expected no error, got %v", errP)
	}

	respP, errP := client.Do(req)
	//fmt.Printf("%#v\n", respP)

	if respP.StatusCode != 200 {
		t.Fatalf("Expected status code 200, got %v", respP.StatusCode)
	}

	for _, cookie := range jar.Cookies(u) {
		fmt.Printf(" 1. Client with a cookie named: %s\n", cookie.Name)
	}

	//fmt.Println(url.QueryEscape(service))

	fmt.Println("===============\n  other access")
	service = srv.URL + "/"
	resp2,err := client.Get(fmt.Sprintf("%s/login?service=%s", authSrv.URL, url.QueryEscape(service)))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("Expected status code 200, got %v", respP.StatusCode)
	}
	// Assert that it was set as expected
	/* if val[0] != "application/json; charset=utf-8" {
	    t.Fatalf("Expected \"application/json; charset=utf-8\", got %s", val[0])
	}*/
}
