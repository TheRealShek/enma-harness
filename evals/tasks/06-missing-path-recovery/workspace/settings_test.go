package appcfg

import "testing"

func TestDefaultPort(t *testing.T) {
	if got := DefaultPort(); got != 8080 {
		t.Fatalf("DefaultPort() = %d, want 8080", got)
	}
}
