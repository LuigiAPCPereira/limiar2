package model

import (
	"testing"
	"time"
)

func TestBoolToInt(t *testing.T) {
	tests := []struct {
		name string
		b    bool
		want int
	}{
		{"true", true, 1},
		{"false", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BoolToInt(tt.b)
			if got != tt.want {
				t.Errorf("BoolToInt(%v) = %d, want %d", tt.b, got, tt.want)
			}
		})
	}
}

func TestParseDBTime(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want time.Time
	}{
		{
			name: "standard format",
			s:    "2024-06-08 15:30:45",
			want: time.Date(2024, 6, 8, 15, 30, 45, 0, time.UTC),
		},
		{
			name: "RFC3339 fallback",
			s:    "2024-06-08T15:30:45Z",
			want: time.Date(2024, 6, 8, 15, 30, 45, 0, time.UTC),
		},
		{
			name: "empty string",
			s:    "",
			want: time.Time{},
		},
		{
			name: "invalid format",
			s:    "not-a-date",
			want: time.Time{},
		},
		{
			name: "partial date",
			s:    "2024-06-08",
			want: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDBTime(tt.s)
			if !got.Equal(tt.want) {
				t.Errorf("ParseDBTime(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
