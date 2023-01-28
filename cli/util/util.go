package cliutil

import (
	"context"

	"github.com/sirupsen/logrus"
	ucli "github.com/urfave/cli/v2"
)

// UtilArgs contains the utility arguments and functions.
type UtilArgs struct {
	// le is the logger entry
	le *logrus.Entry
	// ctx is the context
	ctx context.Context

	// ObjectRef is the object ref string.
	ObjectRef string
}

// BuildFlags attaches the flags to a flag set.
func (a *UtilArgs) BuildFlags() []ucli.Flag {
	return []ucli.Flag{}
}

// BuildCommands attaches the commands.
func (a *UtilArgs) BuildCommands() []*ucli.Command {
	return []*ucli.Command{{
		Name:   "parse-object-ref",
		Usage:  "parses an object.ObjectRef and prints information",
		Action: a.RunParseObjectRef,
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:        "object-ref",
				Aliases:     []string{"r"},
				Usage:       "reference to parse",
				Destination: &a.ObjectRef,
			},
		},
	}}
}

// SetContext sets the context.
func (a *UtilArgs) SetContext(c context.Context) {
	a.ctx = c
}

// GetContext returns the context.
func (a *UtilArgs) GetContext() context.Context {
	if c := a.ctx; c != nil {
		return c
	}
	return context.TODO()
}

// SetLogger sets the root log entry.
func (a *UtilArgs) SetLogger(le *logrus.Entry) {
	a.le = le
}

// GetLogger returns the log entry
func (a *UtilArgs) GetLogger() *logrus.Entry {
	if le := a.le; le != nil {
		return le
	}
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	return logrus.NewEntry(log)
}
