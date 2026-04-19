package data

import (
	"strings"
	"testing"

	"caichip/internal/biz"
)

func TestBuildExtractPrompt_UserDescriptionOrder(t *testing.T) {
	t.Parallel()
	p := buildExtractPrompt("M1", "ACME", &biz.HsDatasheetAssetRecord{LocalPath: ""}, "line1\nline2")
	if !strings.HasPrefix(p, "MODEL: M1\nMANUFACTURER: ACME\nDATASHEET_DATA:") {
		t.Fatalf("unexpected prefix: %q", p)
	}
	if !strings.Contains(p, "USER_DESCRIPTION:") {
		t.Fatalf("missing USER_DESCRIPTION: %q", p)
	}
	if !strings.Contains(p, "line1") {
		t.Fatalf("expected sanitized user text in prompt: %q", p)
	}
}
