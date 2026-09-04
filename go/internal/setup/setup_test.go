package setup

import (
	"strings"
	"testing"
)

func TestMigrationSQLForPGXRemovesOnErrorStopDirective(t *testing.T) {
	got, err := migrationSQLForPGX([]byte("\\set ON_ERROR_STOP on\r\nCREATE TABLE example (id integer);\r\n"))
	if err != nil {
		t.Fatalf("migrationSQLForPGX returned error: %v", err)
	}
	if strings.Contains(got, `\set`) {
		t.Fatalf("psql directive was not removed: %q", got)
	}
	if !strings.Contains(got, "CREATE TABLE example") {
		t.Fatalf("SQL statement was removed: %q", got)
	}
}

func TestMigrationSQLForPGXRejectsUnknownPSQLDirective(t *testing.T) {
	if _, err := migrationSQLForPGX([]byte("\\i another.sql\nSELECT 1;")); err == nil {
		t.Fatal("unknown psql directive was accepted")
	}
}
