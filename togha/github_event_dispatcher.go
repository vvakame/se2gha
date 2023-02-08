package togha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"os"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/vvakame/se2gha/log"
	"golang.org/x/sync/errgroup"
)

type EventDispatcher interface {
	Dispatch(ctx context.Context, req DispatchRequest) error
}

type ReceiverRepo struct {
	Owner string
	Name  string
}

type DispatchRequest interface {
	EventType() (string, error)
	Payload() (json.RawMessage, error)
}

type EventDispatcherConfig struct {
	GitHubClient  *github.Client
	ReceiverRepos []*ReceiverRepo
}

func NewEventDispatcher(ctx context.Context, cfg *EventDispatcherConfig) (EventDispatcher, error) {
	if cfg == nil {
		cfg = &EventDispatcherConfig{}
	}
	if cfg.GitHubClient == nil {
		ghaRepoToken := os.Getenv("GHA_REPO_TOKEN")
		if ghaRepoToken == "" {
			return nil, errors.New("GHA_REPO_TOKEN environment variable is required")
		}

		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: ghaRepoToken},
		)
		tc := oauth2.NewClient(ctx, ts)
		client := github.NewClient(tc)

		cfg.GitHubClient = client
	}
	if cfg.ReceiverRepos == nil {
		ghaRepos := os.Getenv("GHA_REPOS")
		if ghaRepos == "" {
			return  nil, errors.New("GHA_REPOS environment variable is required")
		}

		repos, err := ParseReceiverRepos(ghaRepos)
		if err != nil {
			return nil, err
		}

		cfg.ReceiverRepos = repos
	}
	if len(cfg.ReceiverRepos) == 0 {
		return nil, errors.New("ReceiverRepos requires over 1 item")
	}

	return &gitHubEventDispatcher{
		ghCli:     cfg.GitHubClient,
		receivers: cfg.ReceiverRepos,
	}, nil
}

type gitHubEventDispatcher struct {
	ghCli     *github.Client
	receivers []*ReceiverRepo
}

func (dsp *gitHubEventDispatcher) Dispatch(ctx context.Context, req DispatchRequest) error {
	eventType, err := req.EventType()
	if err != nil {
		return err
	}
	payload, err := req.Payload()
	if err != nil {
		return err
	}

	log.Debugf(ctx, "github dispatch event: %s, %s", eventType, string(payload))

	var eg errgroup.Group
	for _, receiver := range dsp.receivers {
		receiver := receiver
		eg.Go(func() error {
			log.Debugf(ctx, "dispatch event to %s/%s", receiver.Owner, receiver.Name)
			_, _, err = dsp.ghCli.Repositories.Dispatch(
				ctx,
				receiver.Owner,
				receiver.Name,
				github.DispatchRequestOptions{
					EventType:     eventType,
					ClientPayload: &payload,
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

func ParseReceiverRepos(reposStr string) ([]*ReceiverRepo, error) {
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
