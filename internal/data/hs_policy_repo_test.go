package data

import (
	"context"
	"testing"
	"time"
)

func TestHsPolicyRepo(t *testing.T) {
	repo := NewHSPolicyRepo(&Data{})
	p, err := repo.LoadByDeclarationDate(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.VersionID == "" {
		t.Fatalf("expect non-empty policy")
	}
}
