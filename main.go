package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type SubscriptionStatus string

const (
	StatusActive   SubscriptionStatus = "active"
	StatusExpired  SubscriptionStatus = "expired"
	StatusCanceled SubscriptionStatus = "canceled"
)

type Subscription struct {
	ID         int
	UserID     int
	Status     SubscriptionStatus
	ExpiresAt  time.Time
	Source     string // e.g. "stripe", "internal"
	ExternalID string // ID in the external system
}

type ReconciliationResult struct {
	Missing  []Subscription // in external source but not in DB
	Stale    []Subscription // status mismatch between DB and external source
	Expired  []Subscription // past ExpiresAt but still marked active in DB
}

type Reconciler struct {
	db *sql.DB
}

func NewReconciler(db *sql.DB) *Reconciler {
	return &Reconciler{db: db}
}

// initSchema creates the subscriptions table if it does not exist.
func (r *Reconciler) initSchema() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS subscriptions (
			id          SERIAL PRIMARY KEY,
			user_id     INT NOT NULL,
			status      TEXT NOT NULL,
			expires_at  TIMESTAMPTZ NOT NULL,
			source      TEXT NOT NULL,
			external_id TEXT NOT NULL UNIQUE
		)
	`)
	return err
}

// fetchActive returns all subscriptions currently marked active in the DB.
func (r *Reconciler) fetchActive() ([]Subscription, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, status, expires_at, source, external_id
		FROM subscriptions
		WHERE status = $1
	`, StatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.Status, &s.ExpiresAt, &s.Source, &s.ExternalID); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// Reconcile compares DB state against an external snapshot and returns discrepancies.
// external is a slice of subscriptions fetched from a payment provider (e.g. Stripe).
func (r *Reconciler) Reconcile(external []Subscription) (ReconciliationResult, error) {
	active, err := r.fetchActive()
	if err != nil {
		return ReconciliationResult{}, fmt.Errorf("fetchActive: %w", err)
	}

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

	return result, nil
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:password@localhost:5432/reconciler?sslmode=disable"
	}

	var db *sql.DB
	var err error

	for i := 0; i < 5; i++ {
		db, err = sql.Open("postgres", dbURL)
		if err == nil && db.Ping() == nil {
			break
		}
		log.Printf("Waiting for database... attempt %d", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal("Could not connect to database:", err)
	}
	defer db.Close()

	rec := NewReconciler(db)

	if err := rec.initSchema(); err != nil {
		log.Fatal("initSchema:", err)
	}

	log.Println("Premium Subscription Reconciler is running...")

	// TODO: replace with a real external provider fetch (e.g. Stripe API)
	external := []Subscription{}

	result, err := rec.Reconcile(external)
	if err != nil {
		log.Fatal("Reconcile:", err)
	}

	log.Printf("Missing: %d  Stale: %d  Expired: %d",
		len(result.Missing), len(result.Stale), len(result.Expired))
}
