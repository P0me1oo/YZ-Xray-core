package core

import "testing"

func TestYZForkVersionMatchesUpstreamVersion(t *testing.T) {
	if got := Version(); got != "26.7.11" {
		t.Fatalf("Version() = %q, want %q", got, "26.7.11")
	}
	if YZForkVersion != "v26.7.11-yz.2" {
		t.Fatalf("YZForkVersion = %q, want %q", YZForkVersion, "v26.7.11-yz.2")
	}
}
