package data

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchFrankfurterRateAt_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"amount":1.0,"base":"USD","date":"2024-05-31","rates":{"CNY":7.2408}}`))
	}))
	t.Cleanup(srv.Close)

	client := srv.Client()
	day := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	r, err := fetchFrankfurterRateAt(context.Background(), client, srv.URL, "USD", "CNY", day)
	if err != nil {
		t.Fatal(err)
	}
	if r < 7.24 || r > 7.241 {
		t.Fatalf("rate got %v want ~7.2408", r)
	}
}

func TestFetchFrankfurterRateAt_SameCCY(t *testing.T) {
	r, err := fetchFrankfurterRateAt(context.Background(), http.DefaultClient, "http://unused", "USD", "USD", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if r != 1 {
		t.Fatalf("got %v", r)
	}
}
