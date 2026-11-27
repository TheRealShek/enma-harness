package core

import "testing"

func TestPercent(t *testing.T) {
	if got := Percent(1, 4); got != 25 {
		t.Fatalf("Percent(1, 4) = %d, want 25", got)
	}
}
