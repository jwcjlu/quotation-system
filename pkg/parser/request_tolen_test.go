package parser

import (
	"context"
	"os"
	"testing"
)

func TestParse_RequestToTolen(t *testing.T) {
	data, err := os.ReadFile("../../Request to Tolen.xlsx")
	if err != nil {
		t.Skip("Request to Tolen.xlsx not found, skipping")
	}
	ctx := context.Background()

	items, err := Parse(ctx, data, ParseModeAuto, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least 1 item")
	}
	t.Logf("parsed %d items", len(items))
	t.Logf("first: model=%q mfr=%q pkg=%q params=%q qty=%d", items[0].Model, items[0].Manufacturer, items[0].Package, items[0].Params, items[0].Quantity)
	t.Logf("row6: model=%q mfr=%q pkg=%q params=%q", items[5].Model, items[5].Manufacturer, items[5].Package, items[5].Params)
}
