package log

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vvakame/sdlog/buildlog"
)

func Debugf(ctx context.Context, format string, a ...interface{}) {
	ctx, err := buildlog.WithConfigurator(ctx, buildlog.DefaultConfigurator)
	if err != nil {
		panic(err)
	}

	logEntry := buildlog.NewLogEntry(ctx, buildlog.WithSourceLocationSkip(4))
	logEntry.Severity = buildlog.SeverityDebug
	logEntry.Message = fmt.Sprintf(format, a...)

	b, err := json.Marshal(logEntry)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}

func Infof(ctx context.Context, format string, a ...interface{}) {
	ctx, err := buildlog.WithConfigurator(ctx, buildlog.DefaultConfigurator)
	if err != nil {
		panic(err)
	}

	logEntry := buildlog.NewLogEntry(ctx, buildlog.WithSourceLocationSkip(4))
	logEntry.Severity = buildlog.SeverityInfo
	logEntry.Message = fmt.Sprintf(format, a...)

	b, err := json.Marshal(logEntry)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}

func Warnf(ctx context.Context, format string, a ...interface{}) {
	ctx, err := buildlog.WithConfigurator(ctx, buildlog.DefaultConfigurator)
	if err != nil {
		panic(err)
	}

	logEntry := buildlog.NewLogEntry(ctx, buildlog.WithSourceLocationSkip(4))
	logEntry.Severity = buildlog.SeverityWarning
	logEntry.Message = fmt.Sprintf(format, a...)

	b, err := json.Marshal(logEntry)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}
