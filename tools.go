package main

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
	"github.com/t-tomalak/logrus-easy-formatter"

	"gopkg.in/ini.v1"
)

/* Session management */

// Status : session Object
type Status struct {
	Lock     bool   `json:"lock"`
	LastSeen int64  `json:"lastseen"`
	Count    int    `json:"count"`
	User     string `json:"user"`
	Confirm  bool   `json:"confirm"`
}

// ToJSONStr Status object to string
func (s *Status) ToJSONStr() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// StrToStatus un serialize to Status object
func StrToStatus(str string) Status {
	var r Status
	json.Unmarshal([]byte(str), &r)
	return r
}

// userFailLimiter : count and manage post requests
func userFailLimiter(s Status, timeLimit int64) Status {
	//var timeLimit int64 = 30
	now := time.Now().UTC().Unix()
	ret := s
	ret.LastSeen = now
	if s.Lock {
		if now-s.LastSeen > timeLimit {
			ret.Lock = false
			ret.Count = 1
		}
	} else {
		ret.Count++
		if now-s.LastSeen > timeLimit {
			ret.Count = 1
		}
		if ret.Count > 3 {
			ret.Lock = true
			ret.Count = 0
		}
	}
	return ret
}

/* Log management */

var log = logrus.New()

func confLog(path string) {
	level := logrus.InfoLevel
	if *debug {
		level = logrus.DebugLevel
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	log = &logrus.Logger{
		Out:   os.Stderr,
		Level: level,
		Formatter: &easy.Formatter{
			TimestampFormat: time.RFC3339,
			LogFormat:       "%lvl% - [%time%] %msg%\n",
		},
	}

	if path != "" && *debug == false {
		writer, _ := rotatelogs.New(
			path+".%Y%m%d%H%M",
			rotatelogs.WithLinkName(path),
			rotatelogs.WithMaxAge(time.Duration(24*365)*time.Hour),
			rotatelogs.WithRotationTime(time.Duration(24)*time.Hour),
		)
		log.SetOutput(writer)
		gin.DefaultWriter = io.MultiWriter(writer)
	}
}

/* Config: read and manage config.ini file */

// Config struct
type Config struct {
	Port           string
	Secret         string
	HashSecret     string
	LdapServer     string
	LdapBind       string
	LogPath        string
	TGCvalidPeriod int
	AdmStatusRead  []string
	AdmStatusDel   []string
}

func readConf(config Config, file string) (Config, error) {
	if _, err := os.Stat(file); err != nil {
		return config, err
	}
	ini.MapTo(&config, file)
	return config, nil
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
