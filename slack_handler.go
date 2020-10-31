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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/vvakame/se2gha/log"
)

var api = slack.New(os.Getenv("SLACK_ACCESS_TOKEN"))

type Request struct {
	Token     string `json:"token"`
	Challenge string `json:"challenge"`
	Type      string `json:"type"`
}

type Response struct {
	Challenge string `json:"challenge"`
}

func eventHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		log.Warnf(ctx, err.Error())
		return
	}
	defer r.Body.Close()

	if s, err := checkSignature(ctx, r.Header, b); err != nil {
		w.WriteHeader(s)
		_, _ = w.Write([]byte(err.Error()))
		log.Warnf(ctx, err.Error())
		return
	}

	req := &Request{}
	err = json.Unmarshal(b, req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Debugf(ctx, "event type: %s", req.Type)
	switch req.Type {
	case "url_verification":
		respJSON := &Response{req.Challenge}
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

		rae, ok := ev.InnerEvent.Data.(*slackevents.ReactionAddedEvent)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			msg := fmt.Sprintf("unexpected event data type1: %T", ev.InnerEvent.Data)
			_, _ = w.Write([]byte(msg))
			log.Warnf(ctx, msg)
			return
		}

		ch, err := api.GetConversationHistoryContext(ctx, &slack.GetConversationHistoryParameters{
			ChannelID: rae.Item.Channel,
			Inclusive: true,
			Latest:    rae.Item.Timestamp,
			Limit:     1,
		})
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			log.Warnf(ctx, err.Error())
			return
		}
		if v := len(ch.Messages); v != 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf("unexpected messages len: %d", v)))
			log.Warnf(ctx, "unexpected messages len: %d", v)
			return
		}

		teamInfo, err := api.GetTeamInfoContext(ctx)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			log.Warnf(ctx, err.Error())
			return
		}

		userProfile, err := api.GetUserProfileContext(ctx, ch.Messages[0].User, false)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			log.Warnf(ctx, err.Error())
			return
		}

		err = dispatchGitHubEvent(ctx, &DispatchGitHubEventRequest{
			SlackEvent:     b,
			SlackEventType: fmt.Sprintf("%s-%s", rae.Type, rae.Reaction),
			SlackUserName:  userProfile.RealName,
			Text:           ch.Messages[0].Text,
			Reaction:       rae.Reaction,
			Link:           fmt.Sprintf("https://%s.slack.com/archives/%s/p%s", teamInfo.Name, rae.Item.Channel, strings.ReplaceAll(rae.Item.Timestamp, ".", "")),
		})
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

func checkSignature(ctx context.Context, header http.Header, body []byte) (int, error) {
	slackSigningSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if slackSigningSecret == "" {
		return http.StatusInternalServerError, errors.New("SLACK_SIGNING_SECRET is empty")
	}

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

	hash := hmac.New(sha256.New, []byte(slackSigningSecret))
	hash.Write(buf.Bytes())
	log.Debugf(ctx, "computed signature: v0=%s", hex.EncodeToString(hash.Sum(nil)))
	if !hmac.Equal(hash.Sum(nil), binarySignature) {
		return http.StatusBadRequest, errors.New("signature mismatch")
	}

	return 0, nil
}
