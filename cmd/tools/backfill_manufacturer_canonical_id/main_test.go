package main

import (
	"reflect"
	"testing"
)

func TestParseTables(t *testing.T) {
	t.Parallel()
	got := parseTables(" a , ,b, ")
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestBackfillOptionsParseSessionAndOverwrite(t *testing.T) {
	opts, err := parseBackfillOptions([]string{"--session-id", "session-1", "--overwrite", "--dry-run"})
	if err != nil {
		t.Fatalf("parseBackfillOptions() error = %v", err)
	}
	if opts.SessionID != "session-1" || !opts.Overwrite || !opts.DryRun {
		t.Fatalf("opts = %+v", opts)
	}
}
