// Package exitplug tries to stop the host process.
package exitplug

import (
	"context"
	"log"
	"net/http"
	"os"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	logger := log.New(os.Stderr, "exitplug: ", 0)
	exit := os.Exit // as a function value too
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/exit":
			os.Exit(3)
		case "/exitvalue":
			exit(4)
		case "/fatal":
			log.Fatalf("fatal %d", 5)
		case "/loggerfatal":
			logger.Fatal("logger", 6)
		case "/defaultfatal":
			log.Default().Fatal("default", 7)
		case "/goexit":
			go os.Exit(8)
		case "/gopanic":
			go func() { panic("boom 9") }()
		}
	}), nil
}
