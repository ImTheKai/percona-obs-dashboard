package obs

import (
	"testing"

	"github.com/percona/obs-dashboard/internal/model"
)

func TestTargetsChangedDetectsDetailsChange(t *testing.T) {
	prev := &model.Package{
		Targets: []model.Target{
			{Repo: "images", Arch: "x86_64", State: "finished"},
		},
	}
	next := &model.Package{
		Targets: []model.Target{
			{Repo: "images", Arch: "x86_64", State: "finished", Details: "succeeded"},
		},
	}

	if !targetsChanged(prev, next) {
		t.Fatal("expected target details change to be detected")
	}
}
