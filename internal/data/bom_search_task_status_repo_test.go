package data

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"caichip/internal/conf"

	"gorm.io/gorm"
)

func TestBOMSearchTaskRepo_ListSearchTaskStatusRows(t *testing.T) {
	repo, db := newBOMSearchTaskStatusRepoTest(t)

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(5 * time.Minute)
	taskID := sql.NullString{String: "task-1", Valid: true}
	if err := db.Create(&BomSearchTask{
		SessionID:         "session-1",
		MpnNorm:           "TPS5430DDA",
		PlatformID:        "hqchip",
		BizDate:           time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		State:             "failed_terminal",
		AutoAttempt:       2,
		ManualAttempt:     1,
		SelectionRevision: 3,
		CaichipTaskID:     taskID,
		LastError:         sql.NullString{String: "timeout", Valid: true},
		CreatedAt:         now,
		UpdatedAt:         now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&CaichipDispatchTask{
		TaskID:          "task-1",
		Queue:           "default",
		ScriptID:        "hqchip",
		Version:         "1.0.0",
		State:           "failed_terminal",
		Attempt:         3,
		RetryMax:        4,
		LeasedToAgentID: sql.NullString{String: "agent-1", Valid: true},
		LeaseDeadlineAt: &deadline,
		ResultStatus:    sql.NullString{String: "failed", Valid: true},
		LastError:       sql.NullString{String: "agent failed", Valid: true},
		CreatedAt:       now,
		UpdatedAt:       now.Add(time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ListSearchTaskStatusRows(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got.SearchTaskID == 0 || got.MpnNorm != "TPS5430DDA" || got.PlatformID != "hqchip" ||
		got.SearchTaskState != "failed_terminal" {
		t.Fatalf("unexpected search row: %+v", got)
	}
	if got.DispatchTaskID != "task-1" || got.DispatchTaskState != "failed_terminal" ||
		got.DispatchResult != "failed" || got.Attempt != 3 || got.RetryMax != 4 ||
		got.DispatchAgentID != "agent-1" || got.LeaseDeadlineAt == nil {
		t.Fatalf("unexpected dispatch fields: %+v", got)
	}
	if got.LastError != "timeout" {
		t.Fatalf("BOM last error should win, got %q", got.LastError)
	}
}

func TestBOMSearchTaskRepo_ListSearchTaskStatusRowsUsesDispatchErrorFallback(t *testing.T) {
	repo, db := newBOMSearchTaskStatusRepoTest(t)

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	taskID := sql.NullString{String: "task-2", Valid: true}
	if err := db.Create(&BomSearchTask{
		SessionID:         "session-1",
		MpnNorm:           "NE555",
		PlatformID:        "icgoo",
		BizDate:           time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		State:             "failed_retryable",
		SelectionRevision: 1,
		CaichipTaskID:     taskID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&CaichipDispatchTask{
		TaskID:       "task-2",
		Queue:        "default",
		ScriptID:     "icgoo",
		Version:      "1.0.0",
		State:        "finished",
		LastError:    sql.NullString{String: "agent failed", Valid: true},
		RequiredTags: []byte("null"),
		ParamsJSON:   []byte("null"),
		ArgvJSON:     []byte("null"),
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ListSearchTaskStatusRows(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LastError != "agent failed" {
		t.Fatalf("expected dispatch last_error fallback, got %+v", rows)
	}
}

func newBOMSearchTaskStatusRepoTest(t *testing.T) (*BOMSearchTaskRepo, *gorm.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := strings.TrimSpace(os.Getenv("TEST_DATABASE_DRIVER"))
	if driver == "" {
		driver = "mysql"
	}
	db, cleanup, err := NewDB(&conf.Data{
		Database: &conf.DataDatabase{
			Driver: driver,
			Dsn:    dsn,
		},
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(cleanup)
	if err := AutoMigrateSchema(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return NewBOMSearchTaskRepo(&Data{DB: db}, nil), db
}
