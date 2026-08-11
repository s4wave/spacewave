//go:build !js

package bldr_project_controller

import (
	"context"
	"strings"
	"testing"

	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/sirupsen/logrus"
)

func TestPublishTargetsRejectsInvalidTargetsBeforeRemoteAccess(t *testing.T) {
	ctrl := &Controller{le: logrus.NewEntry(logrus.New())}
	ctrl.conf.Store(&Config{ProjectConfig: &bldr_project.ProjectConfig{
		Publish: map[string]*bldr_project.PublishConfig{"release": {}},
	}})

	tests := []struct {
		name    string
		targets []string
		want    string
	}{
		{name: "omitted", targets: nil, want: "no targets"},
		{name: "whitespace", targets: []string{"   "}, want: "must not be empty"},
		{name: "mixed empty", targets: []string{"release", ""}, want: "must not be empty"},
		{name: "unknown", targets: []string{"relase"}, want: "unknown publish target: relase"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ctrl.PublishTargets(context.Background(), "source", test.targets, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
