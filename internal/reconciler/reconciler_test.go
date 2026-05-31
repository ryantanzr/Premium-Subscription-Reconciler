package reconciler

import (
	"testing"
	"time"
)

func TestReconcile_DetectsExpired(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	active := []Subscription{
		{ID: 1, UserID: 10, Status: StatusActive, ExpiresAt: past, ExternalID: "ext_1"},
		{ID: 2, UserID: 11, Status: StatusActive, ExpiresAt: future, ExternalID: "ext_2"},
	}

	result := reconcileLogic(active, nil)

	if len(result.Expired) != 1 {
		t.Errorf("expected 1 expired, got %d", len(result.Expired))
	}
	if result.Expired[0].ExternalID != "ext_1" {
		t.Errorf("wrong subscription flagged as expired")
	}
}

func TestReconcile_DetectsMissing(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)

	active := []Subscription{}
	external := []Subscription{
		{ExternalID: "ext_99", Status: StatusActive, ExpiresAt: future},
	}

	result := reconcileLogic(active, external)

	if len(result.Missing) != 1 {
		t.Errorf("expected 1 missing, got %d", len(result.Missing))
	}
}

func TestReconcile_DetectsStale(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)

	active := []Subscription{
		{ID: 1, ExternalID: "ext_1", Status: StatusActive, ExpiresAt: future},
	}
	external := []Subscription{
		{ExternalID: "ext_1", Status: StatusCanceled, ExpiresAt: future},
	}

	result := reconcileLogic(active, external)

	if len(result.Stale) != 1 {
		t.Errorf("expected 1 stale, got %d", len(result.Stale))
	}
}
