package service

import (
	"testing"

	v1 "caichip/api/agent/v1"
	"caichip/internal/biz"
)

func TestNeedsScriptSync(t *testing.T) {
	pub := &biz.PublishedScriptMeta{Version: "1.2.3", SHA256: "aa"}
	if !needsScriptSync(pub, nil) {
		t.Fatal("nil row should need sync")
	}
	if needsScriptSync(pub, &v1.ScriptRow{ScriptId: "x", Version: "v1.2.3"}) {
		t.Fatal("same version no sha should not need sync")
	}
	bb := "bb"
	if !needsScriptSync(pub, &v1.ScriptRow{ScriptId: "x", Version: "1.2.3", PackageSha256: &bb}) {
		t.Fatal("sha mismatch should need sync")
	}
}

func TestBuildDownloadURL_Service(t *testing.T) {
	u := buildDownloadURL("", "/static/agent-scripts", "p/s/v/z.zip")
	if u != "/static/agent-scripts/p/s/v/z.zip" {
		t.Fatalf("got %q", u)
	}
	u2 := buildDownloadURL("https://ex.com", "/static/agent-scripts", "p/s/v/z.zip")
	if u2 != "https://ex.com/static/agent-scripts/p/s/v/z.zip" {
		t.Fatalf("got %q", u2)
	}
}
