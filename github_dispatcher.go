package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/go-github/v32/github"
	"github.com/vvakame/se2gha/log"
	"golang.org/x/oauth2"
)

type DispatchGitHubEventRequest struct {
	SlackEvent     json.RawMessage `json:"slack_event"`
	SlackEventType string          `json:"slack_event_type"`
	SlackUserName  string          `json:"slack_user_name"`
	Text           string          `json:"text"`
	Reaction       string          `json:"reaction"`
	Link           string          `json:"link"`
}

func dispatchGitHubEvent(ctx context.Context, req *DispatchGitHubEventRequest) error {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GHA_REPO_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	log.Debugf(ctx, "github dispatch event: %s", string(b))

	clientPayload := json.RawMessage(b)
	for _, receiver := range receivers {
		log.Debugf(ctx, "dispatch event to %s/%s", receiver.Owner, receiver.Name)
		_, _, err = client.Repositories.Dispatch(
			ctx,
			receiver.Owner,
			receiver.Name,
			github.DispatchRequestOptions{
				EventType:     fmt.Sprintf("slack-event-%s", req.SlackEventType),
				ClientPayload: &clientPayload,
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}
