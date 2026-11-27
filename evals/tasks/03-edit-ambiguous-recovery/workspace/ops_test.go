package ops

import "testing"

func TestOps(t *testing.T) {
	if Alpha() != 0 || Beta() != 2 {
		t.Fatalf("got (%d, %d), want (0, 2)", Alpha(), Beta())
	}
}
