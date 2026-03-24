package data

import (
	"os"
	"testing"

	"caichip/internal/conf"
)

func TestDatabasePing(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := os.Getenv("TEST_DATABASE_DRIVER")
	if driver == "" {
		driver = "mysql"
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
		t.Fatal("expected non-nil db")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatal(err)
	}
}
