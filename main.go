package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"

	_ "github.com/slack-go/slack"
)

var receivers []*ReceiverRepo

type ReceiverRepo struct {
	Owner string
	Name  string
}

func main() {
	{
		repos, err := parseReceiverRepos(os.Getenv("GHA_REPOS"))
		if err != nil {
			log.Fatal(err)
		}

		receivers = repos
	}

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

func parseReceiverRepos(reposStr string) ([]*ReceiverRepo, error) {
	reposStr = strings.TrimSpace(reposStr)
	ss1 := strings.Split(reposStr, ",")

	var repos []*ReceiverRepo
	for _, s := range ss1 {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		ss2 := strings.SplitN(s, "/", 2)
		if len(ss2) != 2 {
			return nil, fmt.Errorf("invalid GHA_REPOS syntax: %s", s)
		}

		repos = append(repos, &ReceiverRepo{
			Owner: ss2[0],
			Name:  ss2[1],
		})
	}

	if len(repos) == 0 {
		return nil, fmt.Errorf("invalid GHA_REPOS: %s", reposStr)
	}

	return repos, nil
}
