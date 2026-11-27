package math

import "testing"

func TestDouble(t *testing.T) {
	if got := Double(5); got != 10 {
		t.Fatalf("Double(5) = %d, want 10", got)
	}
}
