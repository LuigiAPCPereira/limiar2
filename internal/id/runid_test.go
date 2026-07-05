package id

import "testing"

func TestNewRunID_Length(t *testing.T) {
	id := NewRunID()
	if len(id) != 16 {
		t.Errorf("NewRunID() length = %d, want 16", len(id))
	}
}

func TestNewRunID_IsHex(t *testing.T) {
	id := NewRunID()
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("NewRunID() contains non-hex char %q in %q", string(c), id)
			break
		}
	}
}

func TestNewRunID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := range 100 {
		id := NewRunID()
		if seen[id] {
			t.Fatalf("NewRunID() produced duplicate ID %q on iteration %d", id, i)
		}
		seen[id] = true
	}
}
