package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/vvakame/se2gha/log"
	"golang.org/x/sync/errgroup"
)

type gitHubEventDispatcher struct {
	ghCli     *github.Client
	receivers []*ReceiverRepo
}

type ReceiverRepo struct {
	Owner string
	Name  string
}

type DispatchGitHubEventRequest struct {
	SlackEvent     json.RawMessage `json:"slack_event"`
	SlackEventType string          `json:"slack_event_type"`

	ReactionAdded *ReactionAddedEventDispatch `json:"reaction_added,omitempty"`
}

type ReactionAddedEventDispatch struct {
	UserName string `json:"user_name"`
	Text     string `json:"text"`
	Reaction string `json:"reaction"`
	Link     string `json:"link"`
}

func (dsp *gitHubEventDispatcher) dispatchGitHubEvent(ctx context.Context, req *DispatchGitHubEventRequest) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	log.Debugf(ctx, "github dispatch event: %s", string(b))

	var eg errgroup.Group
	clientPayload := json.RawMessage(b)
	for _, receiver := range dsp.receivers {
		receiver := receiver
		eg.Go(func() error {
			log.Debugf(ctx, "dispatch event to %s/%s", receiver.Owner, receiver.Name)
			_, _, err = dsp.ghCli.Repositories.Dispatch(
				ctx,
				receiver.Owner,
				receiver.Name,
				github.DispatchRequestOptions{
					EventType:     fmt.Sprintf("slack-event-%s", req.SlackEventType),
					ClientPayload: &clientPayload,
				},
			)
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
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
