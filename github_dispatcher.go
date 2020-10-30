package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

type DispatchGitHubEventRequest struct {
	SlackEvent     json.RawMessage `json:"slackEvent"`
	SlackEventType string          `json:"slackEventType"`
	SlackUserName  string          `json:"slackUserName"`
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
	Debugf(ctx, "github dispatch event: %s", string(b))

	clientPayload := json.RawMessage(b)
	repo, resp, err := client.Repositories.Dispatch(
		ctx,
		os.Getenv("GHA_REPO_OWNER"),
		os.Getenv("GHA_REPO_NAME"),
		github.DispatchRequestOptions{
			EventType:     fmt.Sprintf("slack-event-%s", req.SlackEventType),
			ClientPayload: &clientPayload,
		},
	)
	if err != nil {
		return err
	}

	Debugf(ctx, "repo: %#v", repo)
	Debugf(ctx, "resp: %#v", resp)

	return nil
}