package app

import "testing"

func TestGreet(t *testing.T) {
	if got := Greet("enma"); got != "hi enma" {
		t.Fatalf("Greet = %q, want %q", got, "hi enma")
	}
}
