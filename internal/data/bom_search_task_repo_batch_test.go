package data

import (
	"encoding/json"
	"testing"

	"caichip/internal/biz"
)

func TestQuoteItemRowsJSONByCacheIDGroupsAndSequences(t *testing.T) {
	got, err := quoteItemRowsJSONByCacheID([]uint64{2, 1, 3}, []quoteItemRow{
		{QuoteID: 1, Model: "A", Manufacturer: "TI", Stock: "10"},
		{QuoteID: 1, Model: "B", Manufacturer: "ADI", Stock: "20"},
		{QuoteID: 2, Model: "C", Manufacturer: "ST", Stock: "30"},
	})
	if err != nil {
		t.Fatalf("quoteItemRowsJSONByCacheID() error = %v", err)
	}
	for _, quoteID := range []uint64{1, 2, 3} {
		if _, ok := got[quoteID]; !ok {
			t.Fatalf("quote id %d missing from result", quoteID)
		}
	}

	var quoteOne []biz.AgentQuoteRow
	if err := json.Unmarshal(got[1], &quoteOne); err != nil {
		t.Fatalf("quote 1 JSON unmarshal: %v", err)
	}
	if len(quoteOne) != 2 {
		t.Fatalf("quote 1 row count = %d, want 2", len(quoteOne))
	}
	if quoteOne[0].Seq != 1 || quoteOne[0].Model != "A" || quoteOne[1].Seq != 2 || quoteOne[1].Model != "B" {
		t.Fatalf("quote 1 rows = %+v", quoteOne)
	}
	if string(got[3]) != "[]" {
		t.Fatalf("quote 3 JSON = %s, want []", string(got[3]))
	}
}
