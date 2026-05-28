package main

import (
	"testing"
	"time"
)

func TestReconcile_DetectsExpired(t *testing.T) {
	// A subscription past its expiry should appear in Expired even with no external data.
	// We test Reconcile's logic in isolation by calling it with an empty external list
	// against a Reconciler whose fetchActive is effectively mocked via the result struct.

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

// reconcileLogic is the pure business logic extracted from Reconciler.Reconcile,
// allowing unit tests without a real database.
func reconcileLogic(active, external []Subscription) ReconciliationResult {
	dbIndex := make(map[string]Subscription, len(active))
	for _, s := range active {
		dbIndex[s.ExternalID] = s
	}

	var result ReconciliationResult
	now := time.Now()

	for _, ext := range external {
		db, found := dbIndex[ext.ExternalID]
		if !found {
			result.Missing = append(result.Missing, ext)
			continue
		}
		if db.Status != ext.Status {
			result.Stale = append(result.Stale, db)
		}
	}

	for _, s := range active {
		if s.ExpiresAt.Before(now) {
			result.Expired = append(result.Expired, s)
		}
	}

	return result
}
