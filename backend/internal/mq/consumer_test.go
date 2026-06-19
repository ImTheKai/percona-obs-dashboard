package mq

import (
	"testing"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
	"github.com/percona/obs-dashboard/internal/store"
)

func TestMergePackageTargetPreservesDetailsForRepeatedState(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	existing := &model.Package{
		Project:      "isv:percona:PR:pr-33:ppg:17",
		Name:         "pg_tde",
		Tags:         []string{"ppg", "pr"},
		RollupState:  model.RollupFinished,
		OKTargets:    0,
		TotalTargets: 1,
		Targets: []model.Target{
			{Repo: "images", Arch: "x86_64", State: "finished", Details: "succeeded"},
		},
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.UpsertPackageState(db, existing, time.Now().UTC()); err != nil {
		t.Fatalf("upsert existing package: %v", err)
	}

	consumer := &Consumer{db: db, root: "isv:percona"}
	merged := consumer.mergePackageTarget(mqMessage{
		Project: "isv:percona:PR:pr-33:ppg:17",
		Package: "pg_tde",
		Repo:    "images",
		Arch:    "x86_64",
	}, model.RollupFinished)

	if len(merged.Targets) != 1 {
		t.Fatalf("expected one target, got %d", len(merged.Targets))
	}
	if merged.Targets[0].Details != "succeeded" {
		t.Fatalf("expected details to be preserved, got %q", merged.Targets[0].Details)
	}
}
