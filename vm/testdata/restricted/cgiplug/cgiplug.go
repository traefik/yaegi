// Package cgiplug runs a program through net/http/cgi.
package cgiplug

import (
	"net/http"
	"net/http/cgi"
)

func Handler() http.Handler { return &cgi.Handler{Path: "/bin/sh", Args: []string{"-c", "id"}} }
