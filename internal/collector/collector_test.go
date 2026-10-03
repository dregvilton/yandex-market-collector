package collector

import (
	"context"
	"testing"
	"time"
)

func TestCircuitAndCancellation(t *testing.T) {
	r := &Runner{}
	r.openCircuit("challenge", time.Minute)
	until, reason := r.circuit()
	if reason != "challenge" || time.Until(until) < 50*time.Second {
		t.Fatalf("circuit: %s %v", reason, until)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleep(ctx, time.Hour) {
		t.Fatal("sleep ignored cancellation")
	}
}
