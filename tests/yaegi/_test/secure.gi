package main

import (
	"net/http"

	"github.com/unrolled/secure" // or "gopkg.in/unrolled/secure.v1"
)

var myHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello world"))
})

func main() {
	secureMiddleware := secure.New(secure.Options{
		AllowedHosts:          []string{"example.com", "ssl.example.com"},
		HostsProxyHeaders:     []string{"X-Forwarded-Host"},
		SSLRedirect:           true,
		SSLHost:               "ssl.example.com",
		SSLProxyHeaders:       map[string]string{"X-Forwarded-Proto": "https"},
		STSSeconds:            315360000,
		STSIncludeSubdomains:  true,
		STSPreload:            true,
		FrameDeny:             true,
		ContentTypeNosniff:    true,
		BrowserXssFilter:      true,
		ContentSecurityPolicy: "script-src $NONCE",
		PublicKey:             `pin-sha256="base64+primary=="; pin-sha256="base64+backup=="; max-age=5184000; includeSubdomains; report-uri="https://www.example.com/hpkp-report"`,
		IsDevelopment: false,
	})

	app := secureMiddleware.Handler(myHandler)
	http.ListenAndServe("127.0.0.1:3000", app)
}
