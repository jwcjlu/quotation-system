package data

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"

	"gorm.io/gorm"
)

func TestDispatchTaskRepo_SubmitLeasedResultFailedSchedulesRetry(t *testing.T) {
	repo, db := newDispatchTaskRepoRetryTest(t)
	taskID, leaseID := seedLeasedDispatchTaskRetryTest(t, db, "task-retry", 1, 3, []int{60, 300, 900})

	before := time.Now()
	if err := repo.SubmitLeasedResult(context.Background(), &biz.TaskResultIn{
		TaskID:       taskID,
		AgentID:      "agent-1",
		LeaseID:      leaseID,
		Status:       "failed",
		ErrorMessage: "proxy rejected",
	}); err != nil {
		t.Fatal(err)
	}

	got := loadDispatchTaskRetryTest(t, db, taskID)
	if got.State != "pending" {
		t.Fatalf("expected pending state, got %+v", got)
	}
	if got.Attempt != 2 {
		t.Fatalf("expected attempt 2, got %+v", got)
	}
	if got.LastError.String != "proxy rejected" || !got.LastError.Valid {
		t.Fatalf("expected last_error to persist failure reason, got %+v", got.LastError)
	}
	if got.NextClaimAt == nil {
		t.Fatal("expected next_claim_at to be set")
	}
	if delta := got.NextClaimAt.Sub(before); delta < 55*time.Second || delta > 65*time.Second {
		t.Fatalf("expected retry delay around 60s, got %v", delta)
	}
	if got.LeaseID.Valid || got.LeasedToAgentID.Valid || got.LeasedAt != nil || got.LeaseDeadlineAt != nil {
		t.Fatalf("expected lease columns to be cleared, got %+v", got)
	}
	if got.ResultStatus.Valid {
		t.Fatalf("did not expect result_status for retry, got %+v", got.ResultStatus)
	}
}

func TestDispatchTaskRepo_SubmitLeasedResultFailedExhaustsToTerminal(t *testing.T) {
	repo, db := newDispatchTaskRepoRetryTest(t)
	taskID, leaseID := seedLeasedDispatchTaskRetryTest(t, db, "task-terminal", 4, 3, []int{60, 300, 900})

	if err := repo.SubmitLeasedResult(context.Background(), &biz.TaskResultIn{
		TaskID:       taskID,
		AgentID:      "agent-1",
		LeaseID:      leaseID,
		Status:       "failed",
		ErrorMessage: "captcha loop",
	}); err != nil {
		t.Fatal(err)
	}

	got := loadDispatchTaskRetryTest(t, db, taskID)
	if got.State != "failed_terminal" {
		t.Fatalf("expected failed_terminal state, got %+v", got)
	}
	if !got.ResultStatus.Valid || got.ResultStatus.String != "failed_terminal" {
		t.Fatalf("expected terminal result_status, got %+v", got.ResultStatus)
	}
	if !got.LastError.Valid || got.LastError.String != "captcha loop" {
		t.Fatalf("expected terminal last_error, got %+v", got.LastError)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}
	if got.NextClaimAt != nil {
		t.Fatalf("did not expect next_claim_at for terminal task, got %+v", got.NextClaimAt)
	}
}

func newDispatchTaskRepoRetryTest(t *testing.T) (*DispatchTaskRepo, *gorm.DB) {
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
	return NewDispatchTaskRepo(&Data{DB: db}, nil), db
}

func seedLeasedDispatchTaskRetryTest(t *testing.T, db *gorm.DB, taskID string, attempt, retryMax int, backoff []int) (string, string) {
	t.Helper()
	leaseID := "lease-" + taskID
	now := time.Now().UTC()
	backoffJSON, err := json.Marshal(backoff)
	if err != nil {
		t.Fatalf("marshal backoff: %v", err)
	}
	row := map[string]interface{}{
		"task_id":             taskID,
		"queue":               "default",
		"script_id":           "retry-demo",
		"version":             "1.0.0",
		"required_tags":       []byte("null"),
		"entry_file":          nil,
		"timeout_sec":         120,
		"params_json":         []byte("null"),
		"argv_json":           []byte("null"),
		"attempt":             attempt,
		"state":               "leased",
		"lease_id":            leaseID,
		"leased_to_agent_id":  "agent-1",
		"leased_at":           now,
		"lease_deadline_at":   now.Add(2 * time.Minute),
		"next_claim_at":       nil,
		"finished_at":         nil,
		"result_status":       nil,
		"last_error":          nil,
		"retry_max":           retryMax,
		"retry_backoff_json":  backoffJSON,
		"created_at":          now,
		"updated_at":          now,
	}
	if err := db.WithContext(context.Background()).Table(TableCaichipDispatchTask).Create(row).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(context.Background()).Where("task_id = ?", taskID).Delete(&CaichipDispatchTask{}).Error
	})
	return taskID, leaseID
}

func loadDispatchTaskRetryTest(t *testing.T, db *gorm.DB, taskID string) CaichipDispatchTask {
	t.Helper()
	var got CaichipDispatchTask
	if err := db.WithContext(context.Background()).Where("task_id = ?", taskID).First(&got).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	return got
}
