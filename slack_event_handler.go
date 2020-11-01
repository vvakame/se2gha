package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/vvakame/se2gha/log"
)

type SlackChallengeRequest struct {
	Token     string `json:"token"`
	Challenge string `json:"challenge"`
	Type      string `json:"type"`
}

type SlackChallengeResponse struct {
	Challenge string `json:"challenge"`
}

type slackEventHandler struct {
	slCli         *slack.Client
	dsp           *gitHubEventDispatcher
	signingSecret string
}

func (h *slackEventHandler) eventHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		log.Warnf(ctx, err.Error())
		return
	}
	defer r.Body.Close()

	if s, err := h.checkSignature(ctx, r.Header, b); err != nil {
		w.WriteHeader(s)
		_, _ = w.Write([]byte(err.Error()))
		log.Warnf(ctx, err.Error())
		return
	}

	req := &SlackChallengeRequest{}
	err = json.Unmarshal(b, req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Debugf(ctx, "event type: %s", req.Type)
	switch req.Type {
	case "url_verification":
		respJSON := &SlackChallengeResponse{req.Challenge}
		resp, err := json.Marshal(respJSON)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(resp)
		return

	case "event_callback":
		log.Debugf(ctx, "event payload: %s", string(b))

		ev, err := slackevents.ParseEvent(
			b,
			slackevents.OptionNoVerifyToken(),
		)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			log.Warnf(ctx, err.Error())
			return
		}

		ghe, err := h.eventCallbackHandler(ctx, b, &ev)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			log.Warnf(ctx, err.Error())
			return
		}

		err = h.dsp.dispatchGitHubEvent(ctx, ghe)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			log.Warnf(ctx, err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
		return

	default:
		log.Debugf(ctx, "event payload: %s", string(b))
	}

	w.WriteHeader(http.StatusOK)
}

func (h *slackEventHandler) eventCallbackHandler(ctx context.Context, original json.RawMessage, ev *slackevents.EventsAPIEvent) (*DispatchGitHubEventRequest, error) {
	switch eventType := ev.InnerEvent.Type; eventType {
	case slackevents.ReactionAdded:
		rae, ok := ev.InnerEvent.Data.(*slackevents.ReactionAddedEvent)
		if !ok {
			return nil, fmt.Errorf("unexpected event data type: %T", ev.InnerEvent.Data)
		}

		return h.reactionAddedEventHandler(ctx, original, ev, rae)

	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}

func (h *slackEventHandler) reactionAddedEventHandler(ctx context.Context, original json.RawMessage, ev *slackevents.EventsAPIEvent, rae *slackevents.ReactionAddedEvent) (*DispatchGitHubEventRequest, error) {
	teamInfo, err := h.slCli.GetTeamInfoContext(ctx)
	if err != nil {
		return nil, err
	}

	msgs, _, _, err := h.slCli.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
		ChannelID: rae.Item.Channel,
		Timestamp: rae.Item.Timestamp,
	})
	if err != nil {
		return nil, err
	}
	if v := len(msgs); v != 1 {
		return nil, fmt.Errorf(fmt.Sprintf("unexpected messages len: %d", v))
	}

	userProfile, err := h.slCli.GetUserProfileContext(ctx, msgs[0].User, false)
	if err != nil {
		return nil, err
	}

	slackName := userProfile.DisplayName
	if slackName == "" {
		slackName = userProfile.RealName
	}
	text := msgs[0].Text
	messageURL := fmt.Sprintf("https://%s.slack.com/archives/%s/p%s", teamInfo.Name, rae.Item.Channel, strings.ReplaceAll(rae.Item.Timestamp, ".", ""))

	return &DispatchGitHubEventRequest{
		SlackEvent:     original,
		SlackEventType: fmt.Sprintf("%s-%s", rae.Type, rae.Reaction),
		ReactionAdded: &ReactionAddedEventDispatch{
			UserName: slackName,
			Text:     text,
			Reaction: rae.Reaction,
			Link:     messageURL,
		},

		SlackUserName: slackName,
		Text:          text,
		Reaction:      rae.Reaction,
		Link:          messageURL,
	}, nil
}

func (h *slackEventHandler) checkSignature(ctx context.Context, header http.Header, body []byte) (int, error) {
	slackRequestTimestamp := header.Get("X-Slack-Request-Timestamp")
	if slackRequestTimestamp == "" {
		return http.StatusBadRequest, errors.New("X-Slack-Request-Timestamp header is required")
	}
	log.Debugf(ctx, "X-Slack-Request-Timestamp: %s", slackRequestTimestamp)

	timestamp, err := strconv.ParseInt(slackRequestTimestamp, 10, 64)
	if err != nil {
		return http.StatusBadRequest, err
	}

	{
		diff := time.Since(time.Unix(timestamp, 0))
		if diff < 0 {
			diff *= -1
		}
		if diff > 5*time.Minute {
			return http.StatusBadRequest, errors.New("too old timestamp")
		}
	}

	slackSignature := header.Get("X-Slack-Signature")
	if slackSignature == "" {
		return http.StatusBadRequest, errors.New("X-Slack-Signature header is required")
	}
	log.Debugf(ctx, "X-Slack-Signature: %s", slackSignature)

	binarySignature, err := hex.DecodeString(strings.TrimPrefix(slackSignature, "v0="))
	if err != nil {
		return http.StatusBadRequest, err
	}

	log.Debugf(ctx, "body length: %d", len(body))

	var buf bytes.Buffer
	buf.WriteString("v0")
	buf.WriteString(":")
	buf.WriteString(slackRequestTimestamp)
	buf.WriteString(":")
	buf.Write(body)

	hash := hmac.New(sha256.New, []byte(h.signingSecret))
	hash.Write(buf.Bytes())
	log.Debugf(ctx, "computed signature: v0=%s", hex.EncodeToString(hash.Sum(nil)))
	if !hmac.Equal(hash.Sum(nil), binarySignature) {
		return http.StatusBadRequest, errors.New("signature mismatch")
	}

	return 0, nil
}
