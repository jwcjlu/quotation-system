package biz

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExpandRunParams(t *testing.T) {
	argv, err := ExpandRunParams(map[string]interface{}{
		"quiet":         true,
		"parse_workers": float64(8),
		"empty_flag":    false,
		"skip":          nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--parse-workers", "8", "--quiet"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("got %v want %v", argv, want)
	}
}

func TestExpandRunParamsJSON(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"parse_workers": 4, "cookies_file": "/a/b.json"})
	argv, err := ExpandRunParamsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if argv[0] != "--cookies-file" || argv[1] != "/a/b.json" {
		t.Fatalf("order/cookies: %v", argv)
	}
	if argv[2] != "--parse-workers" || argv[3] != "4" {
		t.Fatalf("parse_workers: %v", argv)
	}
}

func TestMergeBOMSearchArgv_defaultWhenEmpty(t *testing.T) {
	argv, err := MergeBOMSearchArgv(nil, "TS5A3159DCKR")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--parse-workers", "8", "--model", "TS5A3159DCKR"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("got %v want %v", argv, want)
	}
}

func TestMergeBOMSearchArgv_usesRunParams(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"parse_workers": 2})
	argv, err := MergeBOMSearchArgv(raw, "X")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--parse-workers", "2", "--model", "X"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("got %v want %v", argv, want)
	}
}
