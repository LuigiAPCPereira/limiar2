package sqlite

import "testing"

func TestIsWindowsDriveLetter(t *testing.T) {
	tests := []struct {
		name   string
		volume string
		want   bool
	}{
		{name: "uppercase drive", volume: "C:", want: true},
		{name: "lowercase drive", volume: "d:", want: true},
		{name: "empty", volume: "", want: false},
		{name: "UNC", volume: `\\server\share`, want: false},
		{name: "single char", volume: "C", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWindowsDriveLetter(tt.volume); got != tt.want {
				t.Fatalf("isWindowsDriveLetter(%q)=%v, want %v", tt.volume, got, tt.want)
			}
		})
	}
}
