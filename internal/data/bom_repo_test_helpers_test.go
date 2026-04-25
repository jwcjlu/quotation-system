package data

import (
	"os"
	"testing"

	"caichip/internal/conf"

	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := os.Getenv("TEST_DATABASE_DRIVER")
	if driver == "" {
		driver = "mysql"
	}
	db, cleanup, err := NewDB(&conf.Data{Database: &conf.DataDatabase{Driver: driver, Dsn: dsn}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&BomLineGap{}, &BomMatchRun{}, &BomMatchResultItem{}); err != nil {
		cleanup()
		t.Fatal(err)
	}
	return db, cleanup
}
