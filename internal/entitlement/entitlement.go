package entitlement

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type Response struct {
	Active        bool       `json:"active"`
	Source        string     `json:"source"`
	ExpiresAt     *time.Time `json:"expiresAt"`
	LastChangedAt time.Time  `json:"lastChangedAt"`
	Reason        string     `json:"reason"`
}

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) InitSchema() error {
	_, err := h.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_entitlements (
			user_id         TEXT PRIMARY KEY,
			active          BOOLEAN NOT NULL DEFAULT FALSE,
			source          TEXT NOT NULL DEFAULT 'NONE',
			expires_at      TIMESTAMPTZ,
			last_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			reason          TEXT NOT NULL DEFAULT 'NO_RECORD'
		)
	`)
	return err
}

// evaluate applies lazy expiration and returns the canonical entitlement state.
// It is a pure function with no DB or HTTP dependency, making it directly testable.
func evaluate(active bool, source string, expiresAt *time.Time, lastChangedAt time.Time, reason string) Response {
	resp := Response{
		Active:        active,
		Source:        source,
		ExpiresAt:     expiresAt,
		LastChangedAt: lastChangedAt,
		Reason:        reason,
	}
	if resp.Active && resp.ExpiresAt != nil && time.Now().After(*resp.ExpiresAt) {
		resp.Active = false
		resp.Source = "NONE"
		resp.Reason = "EXPIRATION"
	}
	return resp
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from /users/<id>/entitlement
	trimmed := strings.TrimPrefix(req.URL.Path, "/users/")
	userID := strings.TrimSuffix(trimmed, "/entitlement")
	if userID == "" || userID == req.URL.Path {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	var (
		active        bool
		source        string
		expiresAt     sql.NullTime
		lastChangedAt time.Time
		reason        string
	)

	err := h.db.QueryRow(`
		SELECT active, source, expires_at, last_changed_at, reason
		FROM user_entitlements
		WHERE user_id = $1
	`, userID).Scan(&active, &source, &expiresAt, &lastChangedAt, &reason)

	var resp Response
	if err == sql.ErrNoRows {
		resp = Response{
			Active:        false,
			Source:        "NONE",
			ExpiresAt:     nil,
			LastChangedAt: time.Now().UTC(),
			Reason:        "NO_RECORD",
		}
	} else if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	} else {
		var exp *time.Time
		if expiresAt.Valid {
			t := expiresAt.Time.UTC()
			exp = &t
		}
		resp = evaluate(active, source, exp, lastChangedAt.UTC(), reason)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
