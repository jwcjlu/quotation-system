package agentapp

import (
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	v1 "caichip/api/agent/v1"

	"google.golang.org/protobuf/types/known/structpb"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testPython(t *testing.T) string {
	t.Helper()
	py := EffectivePython("")
	if _, err := exec.LookPath(py); err != nil {
		t.Skipf("Python 解释器不可用: %q (%v)", py, err)
	}
	return py
}

func TestTail(t *testing.T) {
	if got := tail("abc", 10); got != "abc" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("a", 100) + "END"
	if got := tail(long, 3); got != "END" {
		t.Fatalf("got %q", got)
	}
}

func TestTruncateOneLine(t *testing.T) {
	if got := truncateOneLine("  a\nb c  ", 100); got != "a b c" {
		t.Fatalf("got %q", got)
	}
	rs := strings.Repeat("世", 5)
	got := truncateOneLine(rs, 2)
	if !strings.HasSuffix(got, "…") || utf8Len(t, got) != 3 {
		t.Fatalf("want 2 runes + ellipsis, got %q (rune len %d)", got, utf8Len(t, got))
	}
}

func utf8Len(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for range s {
		n++
	}
	return n
}

func TestFindVersionRoot(t *testing.T) {
	dir := t.TempDir()
	_, err := findVersionRoot(dir, "missing", "1.0.0")
	if err == nil {
		t.Fatal("expected error for missing script dir")
	}
	if err := os.MkdirAll(filepath.Join(dir, "demo", "1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := findVersionRoot(dir, "demo", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) != "1.0.0" {
		t.Fatalf("got %s", root)
	}
}

func TestResolveEntry(t *testing.T) {
	root := t.TempDir()
	if got := resolveEntry(root, ""); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
	mainPy := filepath.Join(root, "main.py")
	if err := os.WriteFile(mainPy, []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveEntry(root, ""); got != mainPy {
		t.Fatalf("resolve default main.py: got %q want %q", got, mainPy)
	}
	if got := resolveEntry(root, "main.py"); got != mainPy {
		t.Fatalf("resolve entry_file: got %q", got)
	}
	parent := filepath.Dir(root)
	sib := filepath.Join(parent, "sibling")
	if err := os.MkdirAll(sib, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(sib, "x.py")
	if err := os.WriteFile(outside, []byte("# y"), 0o644); err != nil {
		t.Fatal(err)
	}
	// root/child/x -> join(sibling) via .. should be rejected (escapes root)
	rel := filepath.Join("..", filepath.Base(sib), "x.py")
	if got := resolveEntry(root, rel); got != "" {
		t.Fatalf("path under root only: got %q", got)
	}
}

func TestRunTask_Skipped(t *testing.T) {
	log := discardLogger()

	file := "icgoo_crawler.py"
	st, code, stdout, msg := RunTask("D:\\tmp", &v1.TaskObject{
		TaskId:    "111111",
		ScriptId:  "icgoo",
		Version:   "0.0.1",
		EntryFile: &file,
		Argv: []string{"--model", "TS5A3159DCKR", "--parse-workers", "8",
			"--user", "18025478083", "--password", "jw123456", "--no-headless"},
		Params:         nil,
		TimeoutSec:     300,
		LeaseId:        "",
		IdempotencyKey: "",
		TraceId:        "",
	}, "python", log)
	if st != "skipped" || code != nil || msg != "missing script_id/version" {
		t.Fatalf("got %v %v %q,%v", st, code, msg, stdout)
	}

}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
