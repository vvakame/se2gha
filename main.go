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

	"github.com/google/go-github/v50/github"
	"github.com/slack-go/slack"
	"golang.org/x/oauth2"

	"go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"

	_ "github.com/slack-go/slack"
)

func main() {
	var ghDispatcher *gitHubEventDispatcher
	{
		ghaRepos := os.Getenv("GHA_REPOS")
		if ghaRepos == "" {
			log.Fatal("GHA_REPOS environment variable is required")
		}
		ghaRepoToken := os.Getenv("GHA_REPO_TOKEN")
		if ghaRepoToken == "" {
			log.Fatal("GHA_REPO_TOKEN environment variable is required")
		}

		repos, err := parseReceiverRepos(ghaRepos)
		if err != nil {
			log.Fatal(err)
		}

		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: ghaRepoToken},
		)
		ctx := context.Background()
		tc := oauth2.NewClient(ctx, ts)
		client := github.NewClient(tc)

		ghDispatcher = &gitHubEventDispatcher{
			ghCli:     client,
			receivers: repos,
		}
	}
	var seHandler *slackEventHandler
	{
		slackAccessToken := os.Getenv("SLACK_ACCESS_TOKEN")
		if slackAccessToken == "" {
			log.Fatal("SLACK_ACCESS_TOKEN environment variable is required")
		}
		slackSigningSecret := os.Getenv("SLACK_SIGNING_SECRET")
		if slackSigningSecret == "" {
			log.Fatal("SLACK_SIGNING_SECRET environment variable is required")
		}

		api := slack.New(slackAccessToken)
		seHandler = &slackEventHandler{
			slCli:         api,
			dsp:           ghDispatcher,
			signingSecret: slackSigningSecret,
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="https://github.com/vvakame/se2gha">se2gha</a>`))
	})
	mux.HandleFunc("/slack/events/action", seHandler.eventHandler)

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
