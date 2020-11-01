package main

import (
	"context"
	"testing"
)

func Test_slackEventHandler_buildSlackURL(t *testing.T) {
	trueV := true
	falseV := false
	tests := []struct {
		name     string
		fragment *slackURLFragment
		want     string
		wantErr  bool
	}{
		{
			name: "message",
			fragment: &slackURLFragment{
				TeamName:      "vvakame",
				ChannelID:     "C01DAR4CQCX",
				Timestamp:     "1604223522001400",
				IsThreadReply: &falseV,
			},
			want:    "https://vvakame.slack.com/archives/C01DAR4CQCX/p1604223522001400",
			wantErr: false,
		},
		{
			name: "message with dot ts",
			fragment: &slackURLFragment{
				TeamName:      "vvakame",
				ChannelID:     "C01DAR4CQCX",
				Timestamp:     "1604223522.001400",
				IsThreadReply: &falseV,
			},
			want:    "https://vvakame.slack.com/archives/C01DAR4CQCX/p1604223522001400",
			wantErr: false,
		},
		{
			name: "message and same thread_ts",
			fragment: &slackURLFragment{
				TeamName:  "vvakame",
				ChannelID: "C01DAR4CQCX",
				Timestamp: "1604223522.001400",
				ThreadTS:  "1604223522.001400",
			},
			want:    "https://vvakame.slack.com/archives/C01DAR4CQCX/p1604223522001400",
			wantErr: false,
		},
		{
			name: "thread reply",
			fragment: &slackURLFragment{
				TeamName:      "vvakame",
				ChannelID:     "C01DAR4CQCX",
				Timestamp:     "1604223522001400",
				ThreadTS:      "1604223531001700",
				IsThreadReply: &trueV,
			},
			want:    "https://vvakame.slack.com/archives/C01DAR4CQCX/p1604223531001700?thread_ts=1604223522.001400",
			wantErr: false,
		},
		{
			name: "thread reply with dot ts",
			fragment: &slackURLFragment{
				TeamName:      "vvakame",
				ChannelID:     "C01DAR4CQCX",
				Timestamp:     "1604223522.001400",
				ThreadTS:      "1604223531.001700",
				IsThreadReply: &trueV,
			},
			want:    "https://vvakame.slack.com/archives/C01DAR4CQCX/p1604223531001700?thread_ts=1604223522.001400",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			h := &slackEventHandler{}
			got, err := h.buildSlackURL(ctx, tt.fragment)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildSlackURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("buildSlackURL() got = %v, want %v", got, tt.want)
			}
		})
	}
}
