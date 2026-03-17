package test

import (
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/home"
)

func TestGreeting(t *testing.T) {
	tests := []struct {
		name string
		hour int
		want string
	}{
		{"early morning", 5, "Good morning"},
		{"mid morning", 9, "Good morning"},
		{"late morning", 11, "Good morning"},
		{"noon", 12, "Good afternoon"},
		{"mid afternoon", 15, "Good afternoon"},
		{"late afternoon", 17, "Good afternoon"},
		{"early evening", 18, "Good evening"},
		{"night", 22, "Good evening"},
		{"midnight", 0, "Good evening"},
		{"late night", 3, "Good evening"},
		{"pre-dawn", 4, "Good evening"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 3, 17, tt.hour, 0, 0, 0, time.Local)
			got := home.Greeting(now)
			if got != tt.want {
				t.Errorf("Greeting at hour %d: got %q, want %q", tt.hour, got, tt.want)
			}
		})
	}
}
