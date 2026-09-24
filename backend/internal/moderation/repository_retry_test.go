package moderation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestRetryDeadlockRetriesWholeTransactionOnlyForMySQL1213(t *testing.T) {
	deadlock := fmt.Errorf("wrapped transaction error: %w", &mysql.MySQLError{Number: 1213, Message: "deadlock"})
	attempts := 0
	if err := retryDeadlock(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return deadlock
		}
		return nil
	}); err != nil || attempts != 3 {
		t.Fatalf("retry result=%v attempts=%d, want success on third attempt", err, attempts)
	}

	attempts = 0
	if err := retryDeadlock(context.Background(), func() error {
		attempts++
		return deadlock
	}); !errors.Is(err, deadlock) || attempts != 5 {
		t.Fatalf("exhaustion result=%v attempts=%d, want original deadlock after five attempts", err, attempts)
	}

	attempts = 0
	other := errors.New("validation failed")
	if err := retryDeadlock(context.Background(), func() error {
		attempts++
		return other
	}); !errors.Is(err, other) || attempts != 1 {
		t.Fatalf("non-deadlock result=%v attempts=%d, want one attempt", err, attempts)
	}
}

func TestRetryDeadlockStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	err := retryDeadlock(ctx, func() error {
		attempts++
		cancel()
		return &mysql.MySQLError{Number: 1213, Message: "deadlock"}
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled result=%v attempts=%d", err, attempts)
	}
}
