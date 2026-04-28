package service

import (
	"context"
	"testing"

	"caichip/internal/biz"
)

func TestCanonicalizeBomImportLines(t *testing.T) {
	svc := &BomService{alias: manufacturerAliasRepoStub{
		"TI": "MFR_TI",
	}}
	lines := []biz.BomImportLine{
		{LineNo: 1, Mpn: "A", Mfr: "TI"},
		{LineNo: 2, Mpn: "B", Mfr: ""},
		{LineNo: 3, Mpn: "C", Mfr: "Unknown"},
	}
	got, err := svc.canonicalizeBomImportLines(context.Background(), lines)
	if err != nil {
		t.Fatalf("canonicalizeBomImportLines() error = %v", err)
	}
	if got[0].ManufacturerCanonicalID == nil || *got[0].ManufacturerCanonicalID != "MFR_TI" {
		t.Fatalf("line 1 canonical = %v, want MFR_TI", got[0].ManufacturerCanonicalID)
	}
	if got[1].ManufacturerCanonicalID != nil {
		t.Fatalf("empty mfr canonical = %v, want nil", got[1].ManufacturerCanonicalID)
	}
	if got[2].ManufacturerCanonicalID != nil {
		t.Fatalf("unknown mfr canonical = %v, want nil pending cleaning", got[2].ManufacturerCanonicalID)
	}
	if lines[0].ManufacturerCanonicalID != nil {
		t.Fatalf("input line canonical mutated = %v, want nil", lines[0].ManufacturerCanonicalID)
	}
}
