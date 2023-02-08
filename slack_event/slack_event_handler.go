package slack_event

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/vvakame/se2gha/log"
	"github.com/vvakame/se2gha/togha"
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
	dsp           togha.EventDispatcher
	signingSecret string
}

type DispatchGitHubEventRequest struct {
	SlackEvent     json.RawMessage `json:"slack_event"`
	SlackEventType string          `json:"slack_event_type"`

	ReactionAdded *ReactionAddedEventDispatch `json:"reaction_added,omitempty"`
}

func (req *DispatchGitHubEventRequest) EventType() (string, error) {
	return fmt.Sprintf("slack-event-%s", req.SlackEventType), nil
}

func (req *DispatchGitHubEventRequest) Payload() (json.RawMessage, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	return b, nil
}

type ReactionAddedEventDispatch struct {
	UserName string `json:"user_name"`
	Text     string `json:"text"`
	Reaction string `json:"reaction"`
	Link     string `json:"link"`
}

func HandleEvent(ctx context.Context, mux *http.ServeMux, dsp togha.EventDispatcher) error {
	slackAccessToken := os.Getenv("SLACK_ACCESS_TOKEN")
	if slackAccessToken == "" {
		return errors.New("SLACK_ACCESS_TOKEN environment variable is required")
	}
	slackSigningSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if slackSigningSecret == "" {
		return errors.New("SLACK_SIGNING_SECRET environment variable is required")
	}

	api := slack.New(slackAccessToken)

	h := &slackEventHandler{
		slCli:         api,
		dsp:           dsp,
		signingSecret: slackSigningSecret,
	}
	mux.HandleFunc("/slack/events/action", h.eventHandler)

	return nil
}

func (h *slackEventHandler) eventHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	b, err := io.ReadAll(r.Body)
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

		err = h.dsp.Dispatch(ctx, ghe)
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
	switch eventType := ev.InnerEvent.Type; slackevents.EventsAPIType(eventType) {
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
	msgs, _, _, err := h.slCli.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
		ChannelID: rae.Item.Channel,
		Timestamp: rae.Item.Timestamp,
	})
	if err != nil {
		return nil, err
	}
	if v := len(msgs); v == 0 {
		return nil, fmt.Errorf(fmt.Sprintf("unexpected messages len: %d", v))
	} else if v != 1 {
		log.Debugf(ctx, "messages len: %d", v)
	}

	userProfile, err := h.slCli.GetUserProfileContext(ctx, &slack.GetUserProfileParameters{
		UserID:        msgs[0].User,
		IncludeLabels: false,
	})
	if err != nil {
		return nil, err
	}

	slackName := userProfile.DisplayName
	if slackName == "" {
		slackName = userProfile.RealName
	}

	msg := msgs[0]
	text := msg.Text
	messageURL, err := h.buildSlackURL(ctx, &slackURLFragment{
		ChannelID: rae.Item.Channel,
		Timestamp: msg.Timestamp,
		ThreadTS:  msg.ThreadTimestamp,
	})
	if err != nil {
		return nil, err
	}

	return &DispatchGitHubEventRequest{
		SlackEvent:     original,
		SlackEventType: fmt.Sprintf("%s-%s", rae.Type, rae.Reaction),
		ReactionAdded: &ReactionAddedEventDispatch{
			UserName: slackName,
			Text:     text,
			Reaction: rae.Reaction,
			Link:     messageURL,
		},
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

type slackURLFragment struct {
	TeamName      string
	ChannelID     string
	Timestamp     string
	ThreadTS      string
	IsThreadReply *bool
}

func (h *slackEventHandler) buildSlackURL(ctx context.Context, fragment *slackURLFragment) (string, error) {
	if fragment.TeamName != "" {
		// ok
	} else {
		teamInfo, err := h.slCli.GetTeamInfoContext(ctx)
		if err != nil {
			return "", err
		}
		fragment.TeamName = teamInfo.Name
	}
	if fragment.ChannelID == "" {
		return "", errors.New("argument ChannelID is required")
	}
	if v := fragment.Timestamp; v != "" && !strings.Contains(v, ".") {
		ts, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return "", err
		}
		t1 := ts / 1000000
		t2 := ts % 1000000
		fragment.Timestamp = fmt.Sprintf("%d.%06d", t1, t2)
	}
	if v := fragment.ThreadTS; v != "" && !strings.Contains(v, ".") {
		ts, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return "", err
		}
		t1 := ts / 1000000
		t2 := ts % 1000000
		fragment.ThreadTS = fmt.Sprintf("%d.%06d", t1, t2)
	}

	if fragment.IsThreadReply != nil {
		if *fragment.IsThreadReply {
			if fragment.Timestamp == "" || fragment.ThreadTS == "" {
				return "", errors.New("argument Timestamp and ThreadTS are required when IsThreadReply == true")
			}
		} else {
			if fragment.ThreadTS != "" {
				return "", errors.New("argument ThreadTS must be empty when IsThreadReply == false")
			}
		}
	} else if fragment.Timestamp != "" && fragment.ThreadTS != "" {
		// ok
	} else {
		msgs, _, _, err := h.slCli.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
			ChannelID: fragment.ChannelID,
			Timestamp: fragment.Timestamp,
		})
		if err != nil {
			return "", err
		}
		if len(msgs) != 1 {
			return "", errors.New("retrieved message length is not 1")
		}
		msg := msgs[0]
		if msg.Timestamp != msg.ThreadTimestamp {
			fragment.Timestamp = msg.Timestamp
			fragment.ThreadTS = msg.ThreadTimestamp
		}
	}

	u := &url.URL{
		Scheme: "https",
	}
	u.Host = fmt.Sprintf("%s.slack.com", fragment.TeamName)
	u.Path = fmt.Sprintf("/archives/%s", fragment.ChannelID)
	if fragment.ThreadTS != "" && fragment.Timestamp != fragment.ThreadTS {
		u.Path += fmt.Sprintf("/p%s", strings.Replace(fragment.ThreadTS, ".", "", 1))
		vs := url.Values{}
		vs.Set("thread_ts", fragment.Timestamp)
		u.RawQuery = vs.Encode()
	} else {
		u.Path += fmt.Sprintf("/p%s", strings.Replace(fragment.Timestamp, ".", "", 1))
	}

	return u.String(), nil
}
