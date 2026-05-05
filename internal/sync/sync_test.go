package sync_test

import (
	"errors"
	"testing"
	"time"

	syncer "degoo-cli/internal/sync"
)

func TestShouldTransfer(t *testing.T) {
	now := time.Now()
	older := now.Add(-1 * time.Hour)
	newer := now.Add(1 * time.Hour)

	tests := []struct {
		sourceMtime time.Time
		destMtime   time.Time
		want        bool
	}{
		{now, time.Time{}, true},    // dest doesn't exist
		{now, older, true},          // source is newer
		{now, now, false},           // same time
		{now, newer, false},         // dest is newer
		{older, now, false},         // source is older
	}
	for _, tt := range tests {
		got := syncer.ShouldTransfer(tt.sourceMtime, tt.destMtime)
		if got != tt.want {
			t.Errorf("ShouldTransfer(source=%v, dest=%v) = %v, want %v",
				tt.sourceMtime, tt.destMtime, got, tt.want)
		}
	}
}

func TestWithRetrySuccess(t *testing.T) {
	calls := 0
	err := syncer.WithRetry(3, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestWithRetryExhausted(t *testing.T) {
	calls := 0
	err := syncer.WithRetry(3, func() error {
		calls++
		return errors.New("always fails")
	})
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}
