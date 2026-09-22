package core

import "testing"

func TestYZForkVersionMatchesUpstreamVersion(t *testing.T) {
	if got := Version(); got != "26.9.9" {
		t.Fatalf("Version() = %q, want %q", got, "26.9.9")
	}
	if YZForkVersion != "v26.8.0" {
		t.Fatalf("YZForkVersion = %q, want %q", YZForkVersion, "v26.8.0")
	}
}
