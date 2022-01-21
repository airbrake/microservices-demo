package main

import (
	"runtime"

	"github.com/airbrake/gobrake/v5"
	"github.com/sirupsen/logrus"
)

// AB - Logrus integration.

type airbrakeHook struct {
	Airbrake *gobrake.Notifier
}

func abLogrusInit(projectID int64, apiKey, env string) *airbrakeHook {
	var hook airbrakeHook

	logrus.SetReportCaller(true)
	n := gobrake.NewNotifier(projectID, apiKey)
	n.AddFilter(func(notice *gobrake.Notice) *gobrake.Notice {
		notice.Context["environment"] = env
		return notice
	})
	hook.Airbrake = n
	return &hook
}

func (hook airbrakeHook) Fire(entry *logrus.Entry) error {
	notice := gobrake.NewNotice(entry.Message, nil, -1)
	if entry.Caller != nil {
		notice.Errors[0].Backtrace = gobrakeBacktrace(entry.Caller)
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

func gobrakeBacktrace(f *runtime.Frame) []gobrake.StackFrame {
	return []gobrake.StackFrame{{
		File: f.File,
		Line: f.Line,
		Func: f.Function,
	}}
}
