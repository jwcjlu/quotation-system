package agentapp

import "testing"

func TestVerifyPythonAndPip_Skip(t *testing.T) {
	cfg := &Config{SkipPythonCheck: true}
	if err := VerifyPythonAndPip(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestEqual(t *testing.T) {
	if !Equal("1.0.0", "v1.0.0") {
		t.Fatal("Equal should normalize v prefix")
	}
	if Equal("1.0.0", "2.0.0") {
		t.Fatal("different versions")
	}
}
