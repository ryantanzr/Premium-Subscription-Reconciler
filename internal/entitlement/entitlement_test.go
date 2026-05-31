package entitlement

import (
	"testing"
	"time"
)

func ptr(t time.Time) *time.Time { return &t }

func TestEvaluate_ActiveWithFutureExpiry(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	resp := evaluate(true, "STORE", ptr(future), time.Now(), "RENEWAL")

	if !resp.Active {
		t.Error("expected active=true for subscription with future expiry")
	}
	if resp.Source != "STORE" {
		t.Errorf("expected source=STORE, got %s", resp.Source)
	}
	if resp.Reason != "RENEWAL" {
		t.Errorf("expected reason=RENEWAL, got %s", resp.Reason)
	}
}

func TestEvaluate_LazyExpiration(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	resp := evaluate(true, "CARRIER", ptr(past), time.Now(), "RENEWAL")

	if resp.Active {
		t.Error("expected active=false after lazy expiration evaluation")
	}
	if resp.Source != "NONE" {
		t.Errorf("expected source=NONE after expiry, got %s", resp.Source)
	}
	if resp.Reason != "EXPIRATION" {
		t.Errorf("expected reason=EXPIRATION, got %s", resp.Reason)
	}
}

func TestEvaluate_InactiveWithPastExpiry_NoChange(t *testing.T) {
	// A row already marked inactive should not be mutated by lazy evaluation.
	past := time.Now().Add(-24 * time.Hour)
	resp := evaluate(false, "NONE", ptr(past), time.Now(), "CANCELED")

	if resp.Active {
		t.Error("expected active=false to remain false")
	}
	if resp.Reason != "CANCELED" {
		t.Errorf("expected original reason=CANCELED to be preserved, got %s", resp.Reason)
	}
}

func TestEvaluate_Sources(t *testing.T) {
	future := time.Now().Add(time.Hour)
	cases := []string{"STORE", "CARRIER", "MARKETPLACE"}
	for _, src := range cases {
		resp := evaluate(true, src, ptr(future), time.Now(), "RENEWAL")
		if resp.Source != src {
			t.Errorf("source %s was mutated to %s", src, resp.Source)
		}
	}
}
