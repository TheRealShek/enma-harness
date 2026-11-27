package pkg

import "testing"

func TestTargetValue(t *testing.T) {
	if got := TargetValue(); got != 42 {
		t.Fatalf("TargetValue() = %d, want 42", got)
	}
}
