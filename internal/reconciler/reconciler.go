package reconciler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type SubscriptionStatus string

const (
	StatusActive   SubscriptionStatus = "active"
	StatusExpired  SubscriptionStatus = "expired"
	StatusCanceled SubscriptionStatus = "canceled"
)

type Subscription struct {
	ID         int                `json:"id"`
	UserID     int                `json:"user_id"`
	Status     SubscriptionStatus `json:"status"`
	ExpiresAt  time.Time          `json:"expires_at"`
	Source     string             `json:"source"`
	ExternalID string             `json:"external_id"`
}

type ReconciliationResult struct {
	Missing []Subscription
	Stale   []Subscription
	Expired []Subscription
}

type Reconciler struct {
	DB *sql.DB
}

func NewReconciler(db *sql.DB) *Reconciler {
	return &Reconciler{DB: db}
}

func (r *Reconciler) InitSchema() error {
	_, err := r.DB.Exec(`
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

func (r *Reconciler) fetchActive() ([]Subscription, error) {
	rows, err := r.DB.Query(`
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

func (r *Reconciler) CreateSubscription(s Subscription) error {
	_, err := r.DB.Exec(`
		INSERT INTO subscriptions (user_id, status, expires_at, source, external_id)
		VALUES ($1, $2, $3, $4, $5)
	`, s.UserID, s.Status, s.ExpiresAt, s.Source, s.ExternalID)
	return err
}

func (r *Reconciler) ListSubscriptions() ([]Subscription, error) {
	rows, err := r.DB.Query(`
		SELECT id, user_id, status, expires_at, source, external_id
		FROM subscriptions
	`)
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
func (r *Reconciler) Reconcile(external []Subscription) (ReconciliationResult, error) {
	active, err := r.fetchActive()
	if err != nil {
		return ReconciliationResult{}, fmt.Errorf("fetchActive: %w", err)
	}
	return reconcileLogic(active, external), nil
}

// reconcileLogic is the pure reconciliation logic, decoupled from the DB for testability.
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

func (r *Reconciler) HandleSubscriptions(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		subs, err := r.ListSubscriptions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(subs)

	case http.MethodPost:
		var s Subscription
		if err := json.NewDecoder(req.Body).Decode(&s); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if err := r.CreateSubscription(s); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
