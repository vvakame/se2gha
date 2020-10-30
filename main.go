package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, world"))
	})
	mux.HandleFunc("/slack/events/action", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		slackSigningSecret := os.Getenv("SLACK_SIGNING_SECRET")
		if slackSigningSecret == "" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("SLACK_SIGNING_SECRET is empty"))
			return
		}

		slackRequestTimestamp := r.Header.Get("X-Slack-Request-Timestamp")
		if slackRequestTimestamp == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("X-Slack-Request-Timestamp header is required"))
			return
		}
		Debugf(ctx, "X-Slack-Request-Timestamp: %s", slackRequestTimestamp)

		timestamp, err := strconv.ParseInt(slackRequestTimestamp, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		{
			diff := time.Since(time.Unix(timestamp, 0))
			if diff < 0 {
				diff *= -1
			}
			if diff > 5*time.Minute {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("too old timestamp"))
				return
			}
		}

		slackSignature := r.Header.Get("X-Slack-Signature")
		if slackSignature == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("X-Slack-Signature header is required"))
			return
		}
		Debugf(ctx, "X-Slack-Signature: %s", slackSignature)

		binarySignature, err := hex.DecodeString(strings.TrimPrefix(slackSignature, "v0="))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		defer r.Body.Close()
		Debugf(ctx, "body length: %d", len(b))

		var buf bytes.Buffer
		buf.WriteString("v0")
		buf.WriteString(":")
		buf.WriteString(slackRequestTimestamp)
		buf.WriteString(":")
		buf.Write(b)

		hash := hmac.New(sha256.New, []byte(slackSigningSecret))
		hash.Write(buf.Bytes())
		Debugf(ctx, "computed signature: v0=%s", hex.EncodeToString(hash.Sum(nil)))
		if !hmac.Equal(hash.Sum(nil), binarySignature) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("signature mismatch"))
			return
		}

		req := &Request{}
		err = json.Unmarshal(b, req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req.Type == "url_verification" {
			respJSON := &Response{req.Challenge}
			resp, err := json.Marshal(respJSON)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(resp)
			return
		}

		Logf(ctx, "event: %s", string(b))

		w.WriteHeader(http.StatusOK)
	})

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
