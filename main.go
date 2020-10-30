package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"

	_ "github.com/slack-go/slack"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, world"))
	})
	mux.HandleFunc("/slack/events/action", eventHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	och := &ochttp.Handler{
		Propagation: &propagation.HTTPFormat{},
		Handler:     mux,
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: och,
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalln("Server closed with error:", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	log.Printf("SIGNAL %d received", <-quit)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Failed to gracefully shutdown: %s", err)
	}
	log.Println("Server shutdown")
}

type Request struct {
	Token     string `json:"token"`
	Challenge string `json:"challenge"`
	Type      string `json:"type"`
}

type Response struct {
	Challenge string `json:"challenge"`
}

// use this when adding new event subscriptions
// https://api.slack.com/events/url_verification
func slackChallengeHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	Logf(r.Context(), string(b))

}
