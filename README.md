# castestserver
A Jasig CAS server for auth tests


## Description

An autonomous and multi-plateform minimalist Jasig CAS server to allow local authentication tests.

All users with **same login and password** are validated.

Now support CASv1 (``/validate``) and CASv2 (``/serviceValidate``). Proxy tickets and user attributes (CASv3) will coming later.

Binaries are downloadable from this github release space.


## Usage

```bash
$ ./castestserver -h
Usage of ./castestserver:
  -basepath string
        basepath
  -port string
        CAS listening port (default "3004")
```


![CAS Session](castestserver.png)


## Useful links

* [CAS Protocol Specification](https://apereo.github.io/cas/5.0.x/protocol/CAS-Protocol-Specification.html)
* [Simple CAS server with ldap binding](https://github.com/jmcarbo/golacas)
* [WIP golang CAS server](https://github.com/apognu/gocas)



## Build


```bash
$ go get github.com/gin-contrib/location
$ go get github.com/gin-contrib/multitemplate
$ go get github.com/gin-gonic/gin
$ go get github.com/gorilla/securecookie
$ go get github.com/robfig/cron

# Add go-bindata to embedded html template
$ go get -u github.com/jteeuwen/go-bindata/...

$ ${GOPATH}/bin/go-bindata -o myweb.go tmpl/

# Build Linux 64
$ GOOS=linux GOARCH=amd64 go build -ldflags "-s" -o castestserver

# Build Windows 32
$ GOOS=windows GOARCH=386 go build  -ldflags "-s" -o castestserver32.exe

# Build MacOs 64
$ GOOS=darwin GOARCH=amd64 go build  -ldflags "-s" -o castestserverOsX

```



## Licence

MIT License

Copyright (c) 2020 Yves Agostini

<yves+github@yvesago.net>
