package board

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestBrochureTypeComesFromTheBytes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
		ok   bool
	}{
		{"pdf", []byte("%PDF-1.4\n\xff\xfe binary"), "application/pdf", true},
		{"pdf with a nul byte", []byte("%PDF-1.4\n\x00"), "application/pdf", true},
		{"text", []byte("Two bedrooms, south facing.\n"), "text/plain; charset=utf-8", true},
		{"utf-8 text", []byte("Дві спальні"), "text/plain; charset=utf-8", true},
		{"%PDF- not at the start", []byte("see %PDF-1.4"), "text/plain; charset=utf-8", true},
		{"%PDF without the dash", []byte("%PDF"), "text/plain; charset=utf-8", true},
		{"empty", nil, "", false},
		{"nul byte", []byte("Two\x00bedrooms"), "", false},
		{"invalid utf-8", []byte("Two \xff bedrooms"), "", false},
		{"jpeg", []byte("\xff\xd8\xff\xe0JFIF"), "", false},
		{"1 MiB of text", bytes.Repeat([]byte("a"), MaxBrochureSize), "text/plain; charset=utf-8", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := BrochureType(tc.data)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("BrochureType = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestBrochurePathIsUserJobBody(t *testing.T) {
	got := BrochurePath("data", "u1", "j1")
	if want := filepath.Join("data", "u1", "j1", "body"); got != want {
		t.Fatalf("BrochurePath = %q, want %q", got, want)
	}
}
