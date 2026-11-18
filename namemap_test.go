package main

import (
	"strings"
	"testing"
)

func TestWhoNameMap(t *testing.T) {
	old := displayNames
	defer func() { displayNames = old }()
	displayNames = parseNameMap("SomeLogin=alice; broken ;=x;y=")
	if got := who("somelogin"); got != "alice" {
		t.Fatalf("mapped login: %q", got)
	}
	if got := who("SOMELOGIN"); got != "alice" {
		t.Fatalf("the mapping must ignore case: %q", got)
	}
	if got := who("someoneelse"); got != "someoneelse" {
		t.Fatalf("an unmapped login must pass through: %q", got)
	}
	if len(displayNames) != 1 {
		t.Fatalf("malformed entries must be ignored: %v", displayNames)
	}
	body := []byte(`{"ref":"refs/heads/main","pusher":{"name":"SomeLogin"},"repository":{"full_name":"owner/repo"},"commits":[{"message":"x"}],"compare":"u"}`)
	line, ok := formatEvent("push", body)
	if !ok || !strings.Contains(line, " alice pushed ") || strings.Contains(strings.ToLower(line), "somelogin") {
		t.Fatalf("the push line should show the mapped name only: %q", line)
	}
}
