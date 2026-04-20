package repositories

import (
	"context"
	"testing"

	"github.com/benenen/myclaw/internal/store"
)

func TestAgentRunRepositoryCreatePendingAndGetByRunID(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	repo := NewAgentRunRepository(db)
	if err := repo.CreatePending(context.Background(), "run_1", "helper-bot", "codex"); err != nil {
		t.Fatalf("CreatePending() error = %v", err)
	}

	run, err := repo.GetByRunID(context.Background(), "run_1")
	if err != nil {
		t.Fatalf("GetByRunID() error = %v", err)
	}
	if run.RunID != "run_1" {
		t.Fatalf("RunID = %q", run.RunID)
	}
	if run.BotName != "helper-bot" {
		t.Fatalf("BotName = %q", run.BotName)
	}
	if run.RuntimeType != "codex" {
		t.Fatalf("RuntimeType = %q", run.RuntimeType)
	}
	if run.Status != "pending" {
		t.Fatalf("Status = %q", run.Status)
	}
}

func TestAgentRunRepositoryUpsertDoneUpdatesExistingPendingRow(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	repo := NewAgentRunRepository(db)
	if err := repo.CreatePending(context.Background(), "run_1", "helper-bot", "codex"); err != nil {
		t.Fatalf("CreatePending() error = %v", err)
	}
	if err := repo.UpsertDone(context.Background(), "run_1", "helper-bot", "codex"); err != nil {
		t.Fatalf("UpsertDone() error = %v", err)
	}

	run, err := repo.GetByRunID(context.Background(), "run_1")
	if err != nil {
		t.Fatalf("GetByRunID() error = %v", err)
	}
	if run.Status != "done" {
		t.Fatalf("Status = %q", run.Status)
	}
}

func TestAgentRunRepositoryUpsertDoneCreatesMissingRow(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	repo := NewAgentRunRepository(db)
	if err := repo.UpsertDone(context.Background(), "run_1", "helper-bot", "codex"); err != nil {
		t.Fatalf("UpsertDone() error = %v", err)
	}

	run, err := repo.GetByRunID(context.Background(), "run_1")
	if err != nil {
		t.Fatalf("GetByRunID() error = %v", err)
	}
	if run.Status != "done" {
		t.Fatalf("Status = %q", run.Status)
	}
}
