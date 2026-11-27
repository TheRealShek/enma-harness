package app

import "testing"

func TestVersion(t *testing.T) {
	if got := Version(); got != "v2" {
		t.Fatalf("Version() = %q, want v2", got)
	}
}
