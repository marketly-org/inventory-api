package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/marketly-org/inventory-api/internal/handler"
	"github.com/marketly-org/inventory-api/internal/store"
)

func main() {
	dsn := os.Getenv("INVENTORY_DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://marketly:marketly@localhost:5432/inventory"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	s, err := store.New(dsn)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		log.Fatalf("failed to init schema: %v", err)
	}

	h := handler.New(s)

	log.Printf("inventory-api starting on :%s", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), h.Routes()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
