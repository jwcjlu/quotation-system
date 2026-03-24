package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestFinalizeSearchTask_MySQL(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run MySQL integration test")
	}
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	db := sqlDB
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	sid := uuid.NewString()
	bizDate := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)
	dateStr := bizDate.Format("2006-01-02")
	platforms, _ := json.Marshal([]string{"ickey"})
	mpn := biz.NormalizeMPNForTask("LM358")

	_, err = db.ExecContext(ctx, `
INSERT INTO bom_session (id, status, biz_date, selection_revision, platform_ids)
VALUES (?, 'draft', ?, 1, ?)`,
		sid, dateStr, platforms)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM bom_search_task WHERE session_id = ?`, sid)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM bom_session WHERE id = ?`, sid)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM bom_quote_cache WHERE mpn_norm = ? AND platform_id = ? AND biz_date = ?`, mpn, "ickey", dateStr)
	})

	_, err = db.ExecContext(ctx, `
INSERT INTO bom_search_task (session_id, mpn_norm, platform_id, biz_date, state, selection_revision, caichip_task_id)
VALUES (?, ?, ?, ?, 'pending', 1, 'cloud-task-1')`,
		sid, mpn, "ickey", dateStr)
	if err != nil {
		t.Fatal(err)
	}

	r := &BOMSearchTaskRepo{db: gormDB, bc: &conf.Bootstrap{}}
	qjson := []byte(`[{"matched_model":"LM358","manufacturer":"TI","unit_price":0.5,"stock":1000}]`)
	err = r.FinalizeSearchTask(ctx, sid, mpn, "ickey", bizDate, "cloud-task-1", "succeeded_quotes", nil, "ok", qjson, nil)
	if err != nil {
		t.Fatal(err)
	}

	var st string
	err = db.QueryRowContext(ctx, `SELECT state FROM bom_search_task WHERE session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?`,
		sid, mpn, "ickey", dateStr).Scan(&st)
	if err != nil {
		t.Fatal(err)
	}
	if st != "succeeded_quotes" {
		t.Fatalf("state=%q", st)
	}

	var outcome string
	var cached []byte
	err = db.QueryRowContext(ctx, `SELECT outcome, quotes_json FROM bom_quote_cache WHERE mpn_norm = ? AND platform_id = ? AND biz_date = ?`,
		mpn, "ickey", dateStr).Scan(&outcome, &cached)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != "ok" {
		t.Fatalf("outcome=%q", outcome)
	}
	if len(cached) == 0 {
		t.Fatal("quotes_json empty")
	}
}
