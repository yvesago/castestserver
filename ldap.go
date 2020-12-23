package main

import (
	"crypto/tls"
	"fmt"
	"github.com/go-ldap/ldap/v3"
)

func ldapValidateUser(username string, password string, config Config) bool {
	log.Debug(fmt.Sprintf("Validate ldap User <%s> <****>", username))

	if username == "" || password == "" {
		return false
	}

	skipVerify := false
	if *debug {
		skipVerify = true
	}

	conn, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", config.LdapServer, 636), &tls.Config{InsecureSkipVerify: skipVerify})
	if err != nil {
		log.Error(err)
		return false
	}
	defer conn.Close()
	binduser := fmt.Sprintf("uid=%s,%s", username, config.LdapBind)
	err = conn.Bind(binduser, password)
	if err != nil {
		log.Debug("[", username, "] ", err)
		return false
	}

	return true
}
