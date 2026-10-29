package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestStepPauseReadsAGoDuration(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Duration
		err   string
	}{
		{"empty", "", 2 * time.Second, ""},
		{"zero", "0", 0, ""},
		{"two seconds", "2s", 2 * time.Second, ""},
		{"bogus", "bogus", 0, "STEP_PAUSE must be a Go duration"},
		{"negative", "-1s", 0, "STEP_PAUSE must not be negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := stepPause(tc.value)
			checkPause(t, got, err, tc.want, tc.err)
		})
	}
}

func TestStepPauseDefaultsWhenUnset(t *testing.T) {
	t.Setenv("STEP_PAUSE", "")
	if err := os.Unsetenv("STEP_PAUSE"); err != nil {
		t.Fatal(err)
	}
	got, err := stepPause(os.Getenv("STEP_PAUSE"))
	checkPause(t, got, err, 2*time.Second, "")
}

func checkPause(t *testing.T, got time.Duration, err error, want time.Duration, wantErr string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("err = %v, want it to contain %q", err, wantErr)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("pause = %s, want %s", got, want)
	}
}
