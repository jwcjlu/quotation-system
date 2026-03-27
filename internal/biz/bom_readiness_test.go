package biz

import "testing"

func TestReadiness_Lenient_AllTerminal(t *testing.T) {
	lines := []LineReadinessSnapshot{{MpnNorm: "ABC"}}
	platforms := []string{"ickey"}
	tasks := []TaskReadinessSnapshot{
		{MpnNorm: "ABC", PlatformID: "ickey", State: "succeeded"},
	}
	if !ReadinessFromTasks(ReadinessLenient, tasks, lines, platforms) {
		t.Fatal("expected ready")
	}
}

func TestReadiness_Lenient_HasPending(t *testing.T) {
	lines := []LineReadinessSnapshot{{MpnNorm: "ABC"}}
	platforms := []string{"ickey"}
	tasks := []TaskReadinessSnapshot{
		{MpnNorm: "ABC", PlatformID: "ickey", State: "pending"},
	}
	if ReadinessFromTasks(ReadinessLenient, tasks, lines, platforms) {
		t.Fatal("expected not ready")
	}
}

func TestReadiness_Strict_NoSucceeded(t *testing.T) {
	lines := []LineReadinessSnapshot{{MpnNorm: "ABC"}}
	platforms := []string{"ickey"}
	tasks := []TaskReadinessSnapshot{
		{MpnNorm: "ABC", PlatformID: "ickey", State: "no_result"},
	}
	if ReadinessFromTasks(ReadinessStrict, tasks, lines, platforms) {
		t.Fatal("expected not ready under strict")
	}
}

func TestReadiness_Strict_HasSucceeded(t *testing.T) {
	lines := []LineReadinessSnapshot{{MpnNorm: "ABC"}}
	platforms := []string{"ickey", "szlcsc"}
	tasks := []TaskReadinessSnapshot{
		{MpnNorm: "ABC", PlatformID: "ickey", State: "no_result"},
		{MpnNorm: "ABC", PlatformID: "szlcsc", State: "succeeded"},
	}
	if !ReadinessFromTasks(ReadinessStrict, tasks, lines, platforms) {
		t.Fatal("expected ready: one platform succeeded covers line")
	}
}
