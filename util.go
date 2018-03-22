package main

import (
"encoding/xml"
)

type CASServiceResponse struct {
	XMLName      xml.Name `xml:"cas:serviceResponse"`
	Xmlns        string   `xml:"xmlns:cas,attr"`
	Success      *CASAuthenticationSuccess
	Failure      *CASAuthenticationFailure
//	ProxySuccess *CASProxySuccess
//	ProxyFailure *CASProxyFailure
}

type CASAuthenticationSuccess struct {
	XMLName xml.Name `xml:"cas:authenticationSuccess"`
	User    CASUser
//	PgtIou  CASPgtIou `xml:",omitempty"`
}

type CASAuthenticationFailure struct {
	XMLName xml.Name `xml:"cas:authenticationFailure"`
	Code    string   `xml:"code,attr"`
	Message string   `xml:",chardata"`
}

type CASUser struct {
	XMLName xml.Name `xml:"cas:user"`
	User    string   `xml:",chardata"`
}

/*type CASPgtIou struct {
	XMLName xml.Name `xml:"cas:proxyGrantingTicket"`
	Ticket  string   `xml:",chardata"`
}*/

func NewCASResponse() CASServiceResponse {
	return CASServiceResponse{
		Xmlns: "http://www.yale.edu/tp/cas",
	}
}

//func NewCASSuccessResponse(u string, pgtiou string) []byte {
func NewCASSuccessResponse(u string) []byte {
	s := NewCASResponse()
	s.Success = &CASAuthenticationSuccess{
		User:   CASUser{User: u},
//		PgtIou: CASPgtIou{Ticket: pgtiou},
	}
	x, _ := xml.Marshal(s)
	return x
}

func NewCASFailureResponse(c string, msg string) []byte {
	f := NewCASResponse()
	f.Failure = &CASAuthenticationFailure{
		Code:    c,
		Message: msg,
	}
	x, _ := xml.Marshal(f)
	return x
}
