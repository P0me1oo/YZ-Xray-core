package core

import "testing"

func TestYZForkVersionMatchesUpstreamVersion(t *testing.T) {
	if got := Version(); got != "26.10.17" {
		t.Fatalf("Version() = %q, want %q", got, "26.10.17")
	}
	if YZForkVersion != "v26.10.0" {
		t.Fatalf("YZForkVersion = %q, want %q", YZForkVersion, "v26.10.0")
	}
}
