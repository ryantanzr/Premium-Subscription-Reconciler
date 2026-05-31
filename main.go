package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rtzr/premium-subscription-reconciler/internal/entitlement"
	"github.com/rtzr/premium-subscription-reconciler/internal/reconciler"
	_ "github.com/lib/pq"
)

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

	rec := reconciler.NewReconciler(db)
	if err := rec.InitSchema(); err != nil {
		log.Fatal("reconciler initSchema:", err)
	}

	ent := entitlement.NewHandler(db)
	if err := ent.InitSchema(); err != nil {
		log.Fatal("entitlement initSchema:", err)
	}

	http.HandleFunc("/subscriptions", rec.HandleSubscriptions)
	http.Handle("/users/", ent)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
