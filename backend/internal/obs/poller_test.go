package obs

import (
	"testing"

	"github.com/percona/obs-dashboard/internal/model"
)

func TestInferScope(t *testing.T) {
	cases := []struct {
		project string
		want    model.Scope
	}{
		{"isv:percona:PR:pr-42:ppg17", model.ScopePR},
		{"isv:percona:ppg:releases:17:containers:ubi9", model.ScopeRelease},
		{"isv:percona:ppg:17:containers:ubi9", model.ScopeContainer},
		{"isv:percona:ppgcommon", model.ScopePPGCommon},
		{"isv:percona:ppg:common", model.ScopeCommon},
		{"isv:percona:ppg:17", model.ScopeVersion},
	}
	for _, c := range cases {
		got := InferScope(c.project)
		if got != c.want {
			t.Errorf("InferScope(%q) = %q, want %q", c.project, got, c.want)
		}
	}
}
