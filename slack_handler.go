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
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var api = slack.New(os.Getenv("SLACK_ACCESS_TOKEN"))

func eventHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	defer r.Body.Close()

	if s, err := checkSignature(ctx, r.Header, b); err != nil {
		w.WriteHeader(s)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	req := &Request{}
	err = json.Unmarshal(b, req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

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
		Logf(ctx, "type: %s, event: %s", req.Type, string(b))

		ev, err := slackevents.ParseEvent(
			b,
			slackevents.OptionNoVerifyToken(),
		)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			Warnf(ctx, err.Error())
			return
		}

		rae, ok := ev.InnerEvent.Data.(*slackevents.ReactionAddedEvent)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			msg := fmt.Sprintf("unexpected event data type1: %T", ev.InnerEvent.Data)
			_, _ = w.Write([]byte(msg))
			Warnf(ctx, msg)
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
			Warnf(ctx, err.Error())
			return
		}
		if v := len(ch.Messages); v != 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf("unexpected messages len: %d", v)))
			Warnf(ctx, "unexpected messages len: %d", v)
			return
		}

		teamInfo, err := api.GetTeamInfoContext(ctx)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			Warnf(ctx, err.Error())
			return
		}

		Logf(ctx, "reaction: %s to %s message", rae.Reaction, ch.Messages[0].Text)
		Logf(ctx, "https://%s.slack.com/archives/%s/p%s", teamInfo.Name, rae.Item.Channel, rae.Item.Timestamp)
		w.WriteHeader(http.StatusOK)
		return

	default:
		Logf(ctx, "type: %s, event: %s", req.Type, string(b))
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
	Debugf(ctx, "X-Slack-Request-Timestamp: %s", slackRequestTimestamp)

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
	Debugf(ctx, "X-Slack-Signature: %s", slackSignature)

	binarySignature, err := hex.DecodeString(strings.TrimPrefix(slackSignature, "v0="))
	if err != nil {
		return http.StatusBadRequest, err
	}

	Debugf(ctx, "body length: %d", len(body))

	var buf bytes.Buffer
	buf.WriteString("v0")
	buf.WriteString(":")
	buf.WriteString(slackRequestTimestamp)
	buf.WriteString(":")
	buf.Write(body)

	hash := hmac.New(sha256.New, []byte(slackSigningSecret))
	hash.Write(buf.Bytes())
	Debugf(ctx, "computed signature: v0=%s", hex.EncodeToString(hash.Sum(nil)))
	if !hmac.Equal(hash.Sum(nil), binarySignature) {
		return http.StatusBadRequest, errors.New("signature mismatch")
	}

	return 0, nil
}
