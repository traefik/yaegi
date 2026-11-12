package main

import (
	"os"

	"github.com/sirupsen/logrus"
)

type Req struct {
	Path string
	Code int
}

func main() {
	l := logrus.New()
	l.SetOutput(os.Stdout)
	l.SetFormatter(&logrus.JSONFormatter{DisableTimestamp: true})
	l.WithFields(logrus.Fields{"req": Req{"/a", 200}, "n": 3}).Info("hello")
	l.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: true})
	l.WithField("req", Req{"/b", 404}).Warn("text")
}
