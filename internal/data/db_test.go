package data

import (
	"os"
	"testing"

	"caichip/internal/conf"
)

func TestMysqlDSNWithParseTime(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"u:p@tcp(h:3306)/db", "u:p@tcp(h:3306)/db?parseTime=true"},
		{"u:p@tcp(h:3306)/db?charset=utf8mb4", "u:p@tcp(h:3306)/db?charset=utf8mb4&parseTime=true"},
		{"u:p@tcp(h:3306)/db?parseTime=true", "u:p@tcp(h:3306)/db?parseTime=true"},
		{"u:p@tcp(h:3306)/db?parseTime=true&charset=utf8mb4", "u:p@tcp(h:3306)/db?parseTime=true&charset=utf8mb4"},
	}
	for _, tt := range tests {
		if got := mysqlDSNWithParseTime(tt.in); got != tt.want {
			t.Errorf("mysqlDSNWithParseTime(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMysqlDSNWithLoc(t *testing.T) {
	tests := []struct {
		dsn, loc, want string
	}{
		{"", "Asia/Shanghai", ""},
		{"u:p@tcp(h:3306)/db?parseTime=true", "", "u:p@tcp(h:3306)/db?parseTime=true"},
		{"u:p@tcp(h:3306)/db?parseTime=true", "Asia/Shanghai", "u:p@tcp(h:3306)/db?parseTime=true&loc=Asia%2FShanghai"},
		{"u:p@tcp(h:3306)/db?parseTime=true&loc=UTC", "Asia/Shanghai", "u:p@tcp(h:3306)/db?parseTime=true&loc=UTC"},
	}
	for _, tt := range tests {
		if got := mysqlDSNWithLoc(tt.dsn, tt.loc); got != tt.want {
			t.Errorf("mysqlDSNWithLoc(%q,%q) = %q, want %q", tt.dsn, tt.loc, got, tt.want)
		}
	}
}

func TestMysqlDSNWithReadWriteTimeout(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"u:p@tcp(h:3306)/db?parseTime=true", "u:p@tcp(h:3306)/db?parseTime=true&readTimeout=60s&writeTimeout=60s"},
		{"u:p@tcp(h:3306)/db?charset=utf8mb4&readTimeout=10s", "u:p@tcp(h:3306)/db?charset=utf8mb4&readTimeout=10s"},
	}
	for _, tt := range tests {
		if got := mysqlDSNWithReadWriteTimeout(tt.in); got != tt.want {
			t.Errorf("mysqlDSNWithReadWriteTimeout(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

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
