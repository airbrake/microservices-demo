package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/airbrake/gobrake/v5"
	"github.com/sirupsen/logrus"
)

// Airbrake global for APM or what ever you need
var Airbrake *gobrake.Notifier

// ABInfo the require information to send data to AB
type ABInfo struct {
	ProjectID   int64
	ProjectKey  string
	Environment string
	SrcPath     string
}

func getAbEnv(target *string, envKey string) {
	v := os.Getenv(envKey)
	if v == "" {
		panic(fmt.Sprintf("environment variable %q not set", envKey))
	}
	*target = v
}

func airbrakeInit() *ABInfo {
	var (
		err       error
		info      ABInfo
		projIDStr string
	)

	getAbEnv(&projIDStr, "AB_PROJECT_ID")
	if info.ProjectID, err = strconv.ParseInt(projIDStr, 10, 64); err != nil {
		panic(fmt.Sprintf("Error converting Airbrake Project id: %s", err))
	}
	getAbEnv(&info.ProjectKey, "AB_PROJECT_KEY")
	getAbEnv(&info.Environment, "AB_ENV")
	info.SrcPath = os.Getenv("AB_SRCPATH")

	Airbrake = gobrake.NewNotifierWithOptions(&gobrake.NotifierOptions{
		ProjectId:   info.ProjectID,
		ProjectKey:  info.ProjectKey,
		Environment: info.Environment,
	})
	return &info
}

// AB - Logrus integration.
type airbrakeHook struct {
	Airbrake *gobrake.Notifier
	srcPath  string
}

func abLogrusInit(info *ABInfo) *airbrakeHook {
	var hook airbrakeHook

	n := gobrake.NewNotifier(info.ProjectID, info.ProjectKey)
	n.AddFilter(func(notice *gobrake.Notice) *gobrake.Notice {
		notice.Context["environment"] = info.Environment
		return notice
	})
	hook.Airbrake = n
	hook.srcPath = info.SrcPath
	return &hook
}

func (hook airbrakeHook) Fire(entry *logrus.Entry) error {
	notice := gobrake.NewNotice(entry.Message, nil, -1)
	if entry.HasCaller() {
		notice.Errors[0].Backtrace = hook.gobrakeBacktrace(entry.Caller)
	}
	notice.Params = asParams(entry.Data)
	notice.Context["severity"] = entry.Level.String()

	hook.Airbrake.SendNoticeAsync(notice)
	return nil
}

func asParams(data logrus.Fields) map[string]interface{} {
	params := make(map[string]interface{}, len(data))
	for k, v := range data {
		switch v := v.(type) {
		case error:
			params[k] = v.Error()
		default:
			params[k] = v
		}
	}
	return params
}

func (hook airbrakeHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
	}
}

// We mess with the file name due to the way we build in docker, the code is
// built from /src so we loose the real github repo location. We reinject it
// by using AB_SRCPATH env var passed in.
func (hook airbrakeHook) gobrakeBacktrace(f *runtime.Frame) []gobrake.StackFrame {
	return []gobrake.StackFrame{{
		File: fmt.Sprintf("%s/%s", hook.srcPath, strings.TrimPrefix(f.File, "/src/")),
		Line: f.Line,
		Func: f.Function,
	}}
}

func abJobStart(ctx context.Context, name string) context.Context {
	ctx, _ = gobrake.NewQueueMetric(ctx, name)
	return ctx
}

func abJobEnd(ctx context.Context, name string, err error) {

	metric := gobrake.ContextQueueMetric(ctx)
	metric.Errored = err != nil
	_ = Airbrake.Queues.Notify(ctx, metric)

	if err != nil {
		notice := gobrake.NewNotice(err, nil, 0)
		notice.Context["queue"] = name
		Airbrake.SendNoticeAsync(notice)
	}
}
