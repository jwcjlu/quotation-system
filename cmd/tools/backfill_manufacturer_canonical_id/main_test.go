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
