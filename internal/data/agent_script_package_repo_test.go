package data

import (
	"context"
	"os"
	"strings"
	"testing"

	"caichip/internal/conf"
)

const agentScriptPackageDDL = `
CREATE TABLE IF NOT EXISTS t_agent_script_package (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    script_id          VARCHAR(128) NOT NULL,
    version            VARCHAR(64)  NOT NULL,
    sha256             CHAR(64)     NOT NULL,
    storage_rel_path   VARCHAR(512) NOT NULL,
    filename           VARCHAR(255) NOT NULL,
    status             VARCHAR(32)  NOT NULL DEFAULT 'uploaded',
    release_notes      TEXT         NULL,
    created_at         DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at         DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_pkg_script_version (script_id, version),
    KEY idx_pkg_script_status (script_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

func TestAgentScriptPackageRepo_PublishRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := os.Getenv("TEST_DATABASE_DRIVER")
	if driver == "" {
		driver = "mysql"
	}
	if strings.ToLower(driver) != "mysql" {
		t.Skip("agent_script_package_repo integration test currently mysql-only DDL")
	}
	c := &conf.Data{
		Database: &conf.DataDatabase{
			Driver: driver,
			Dsn:    dsn,
		},
	}
	db, cleanup, err := NewDB(c)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if db == nil {
		t.Fatal("expected db")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS t_agent_script_package"); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(context.Background(), agentScriptPackageDDL); err != nil {
		t.Fatal(err)
	}

	d := &Data{DB: db, dbDriver: driver}
	repo := NewAgentScriptPackageRepo(d)
	ctx := context.Background()

	id, err := repo.Insert(ctx, &AgentScriptPackage{
		ScriptID:       "findchips",
		Version:        "1.0.0",
		SHA256:         "ab" + strings.Repeat("0", 62),
		StorageRelPath: "findchips/1.0.0/pkg.zip",
		Filename:       "pkg.zip",
		Status:         "uploaded",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("insert id: %d", id)
	}
	if err := repo.SetPublished(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetPublished(ctx, "findchips")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected published row")
	}
	if got.Version != "1.0.0" {
		t.Fatalf("version: got %q", got.Version)
	}
	if !strings.EqualFold(got.Status, "published") {
		t.Fatalf("status: %q", got.Status)
	}
}
