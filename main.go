package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vvakame/se2gha/kintone_event"
	"github.com/vvakame/se2gha/slack_event"
	"github.com/vvakame/se2gha/togha"
	"go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"

	_ "github.com/slack-go/slack"
)

func main() {
	ctx := context.Background()

	dsp, err := togha.NewEventDispatcher(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="https://github.com/vvakame/se2gha">se2gha</a>`))
	})

	err = slack_event.HandleEvent(ctx, mux, dsp)
	if err != nil {
		log.Fatal(err)
	}

	err = kintone_event.HandleEvent(ctx, mux, dsp)
	if err != nil {
		log.Fatal(err)
	}

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
