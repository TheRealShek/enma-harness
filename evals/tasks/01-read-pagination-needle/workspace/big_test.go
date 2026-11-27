package bigpkg

import "testing"

func TestMultiply(t *testing.T) {
	if got := Multiply(6, 7); got != 42 {
		t.Fatalf("Multiply(6, 7) = %d, want 42", got)
	}
}
