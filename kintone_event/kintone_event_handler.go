package kintone_event

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/vvakame/se2gha/log"
	"github.com/vvakame/se2gha/togha"
)

type eventHandler struct {
	dsp togha.EventDispatcher
}

type DispatchGitHubEventRequest struct {
	Event    *KintoneEvent   `json:"kintone_event"`
	EventRaw json.RawMessage `json:"kintone_event_raw"`
}

func (req *DispatchGitHubEventRequest) EventType() (string, error) {
	return fmt.Sprintf("kintone-event-%s-%s", req.Event.App.ID, req.Event.Type), nil
}

func (req *DispatchGitHubEventRequest) Payload() (json.RawMessage, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// https://jp.cybozu.help/k/ja/user/app_settings/set_webhook/webhook_notification.html
type KintoneEvent struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	App         *KintoneApp     `json:"app"`
	Record      json.RawMessage `json:"record"`
	RecordTitle string          `json:"recordTitle"`
	URL         string          `json:"url"`
}

type KintoneApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func HandleEvent(ctx context.Context, mux *http.ServeMux, dsp togha.EventDispatcher) error {
	h := &eventHandler{
		dsp: dsp,
	}
	mux.HandleFunc("/kintone/events/action", h.eventHandler)

	return nil
}

func (h *eventHandler) eventHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		log.Warnf(ctx, err.Error())
		return
	}
	defer r.Body.Close()

	req := &KintoneEvent{}
	err = json.Unmarshal(b, req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Debugf(ctx, "event type: %s", req.Type)
	log.Debugf(ctx, "event payload: %s", string(b))

	err = h.dsp.Dispatch(ctx, &DispatchGitHubEventRequest{
		EventRaw: b,
		Event:    req,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		log.Warnf(ctx, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
}
