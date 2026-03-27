package data

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"
)

func TestBOMSessionCreateGet(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := os.Getenv("TEST_DATABASE_DRIVER")
	if driver == "" {
		driver = "mysql"
	}
	bc := &conf.Bootstrap{
		Data: &conf.Data{
			Database: &conf.DataDatabase{
				Driver: driver,
				Dsn:    dsn,
			},
		},
	}
	d, cleanup, err := NewData(bc)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if d == nil || d.DB == nil {
		t.Fatal("expected database")
	}
	repo := NewBOMSessionRepo(d)
	ctx := context.Background()
	now := time.Now().UTC()
	bizDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	s := &biz.BOMSession{
		Title:             "wire-test",
		Status:            "draft",
		BizDate:           bizDate,
		SelectionRevision: 1,
		PlatformIDs:       []string{"szlcsc", "ickey"},
	}
	if err := repo.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Fatal("expected session id")
	}
	got, err := repo.GetByID(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != s.Title || got.Status != s.Status || got.SelectionRevision != s.SelectionRevision {
		t.Fatalf("mismatch: %+v vs %+v", got, s)
	}
	if len(got.PlatformIDs) != 2 {
		t.Fatalf("platforms: %v", got.PlatformIDs)
	}
}

func TestReplaceSessionLines(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := os.Getenv("TEST_DATABASE_DRIVER")
	if driver == "" {
		driver = "mysql"
	}
	bc := &conf.Bootstrap{
		Data: &conf.Data{
			Database: &conf.DataDatabase{
				Driver: driver,
				Dsn:    dsn,
			},
		},
	}
	d, cleanup, err := NewData(bc)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if d == nil || d.DB == nil {
		t.Fatal("expected database")
	}
	repo := NewBOMSessionRepo(d)
	ctx := context.Background()
	now := time.Now().UTC()
	bizDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	s := &biz.BOMSession{
		Title:             "line-test",
		Status:            "draft",
		BizDate:           bizDate,
		SelectionRevision: 1,
		PlatformIDs:       []string{},
	}
	if err := repo.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	q1 := 10.0
	q2 := 20.0
	lines := []*biz.BOMSessionLine{
		{LineNo: 1, RawText: "a", MPN: "LM358", MFR: "TI", Package: "SOP", Qty: &q1},
		{LineNo: 2, RawText: "b", MPN: "STM32", MFR: "", Package: "", Qty: &q2},
	}
	if err := repo.ReplaceSessionLines(ctx, s.ID, "auto", lines); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.DB.WithContext(ctx).Raw(`SELECT COUNT(*) FROM bom_session_line WHERE session_id = ?`, s.ID).Row().Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 lines, got %d", n)
	}
	if err := repo.ReplaceSessionLines(ctx, s.ID, "auto", []*biz.BOMSessionLine{}); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.WithContext(ctx).Raw(`SELECT COUNT(*) FROM bom_session_line WHERE session_id = ?`, s.ID).Row().Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 lines after replace with empty, got %d", n)
	}
}

func TestDeleteSessionLineReorder(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := os.Getenv("TEST_DATABASE_DRIVER")
	if driver == "" {
		driver = "mysql"
	}
	bc := &conf.Bootstrap{
		Data: &conf.Data{
			Database: &conf.DataDatabase{
				Driver: driver,
				Dsn:    dsn,
			},
		},
	}
	d, cleanup, err := NewData(bc)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	repo := NewBOMSessionRepo(d)
	ctx := context.Background()
	now := time.Now().UTC()
	bizDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	s := &biz.BOMSession{
		Title:             "del-reorder",
		Status:            "draft",
		BizDate:           bizDate,
		SelectionRevision: 1,
		PlatformIDs:       []string{"ickey"},
	}
	if err := repo.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	q := 1.0
	for i := 1; i <= 3; i++ {
		ln := &biz.BOMSessionLine{MPN: fmt.Sprintf("MPN%d", i), Qty: &q}
		if err := repo.InsertSessionLine(ctx, s.ID, ln); err != nil {
			t.Fatal(err)
		}
	}
	var midID int64
	if err := d.DB.WithContext(ctx).Raw(`SELECT id FROM bom_session_line WHERE session_id = ? AND line_no = 2`, s.ID).Row().Scan(&midID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteSessionLine(ctx, s.ID, midID); err != nil {
		t.Fatal(err)
	}
	var nos []int
	rows, err := d.DB.WithContext(ctx).Raw(`SELECT line_no FROM bom_session_line WHERE session_id = ? ORDER BY line_no`, s.ID).Rows()
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		nos = append(nos, n)
	}
	rows.Close()
	if len(nos) != 2 || nos[0] != 1 || nos[1] != 2 {
		t.Fatalf("expected line_no 1,2 got %v", nos)
	}
}
