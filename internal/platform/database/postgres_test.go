package database

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPingWithRetry(t *testing.T) {
	calls := 0
	ping := func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("connection refused")
		}
		return nil
	}

	if err := pingWithRetry(context.Background(), ping); err != nil || calls != 3 {
		t.Fatalf("err = %v, calls = %d", err, calls)
	}
}

func TestPingWithRetryGivesUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := pingWithRetry(ctx, func(context.Context) error { return errors.New("connection refused") })
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}
