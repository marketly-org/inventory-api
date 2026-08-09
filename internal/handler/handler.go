package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/marketly-org/inventory-api/internal/models"
	"github.com/marketly-org/inventory-api/internal/store"
)

type Handler struct {
	store *store.Store
}

func New(s *store.Store) *Handler {
	return &Handler{store: s}
}

func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/products", h.handleProducts)
	mux.HandleFunc("/products/", h.handleProduct)
	mux.HandleFunc("/reserve", h.Reserve)
	mux.HandleFunc("/confirm", h.Confirm)
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/ready", h.Ready)
	return mux
}

func (h *Handler) handleProducts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	products, err := h.store.ListProducts(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": products})
}

func (h *Handler) handleProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sku := r.URL.Path[len("/products/"):]
	if sku == "" {
		http.Error(w, "sku required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	product, err := h.store.GetProduct(ctx, sku)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h *Handler) Reserve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.ReserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SKU == "" || req.Quantity <= 0 {
		http.Error(w, "sku and positive quantity required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.store.Reserve(ctx, req.SKU, req.Quantity); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrInsufficient) {
			http.Error(w, "insufficient stock", http.StatusConflict)
			return
		}
		http.Error(w, fmt.Sprintf("reserve failed: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reserved"})
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.ConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SKU == "" || req.Quantity <= 0 {
		http.Error(w, "sku and positive quantity required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.store.Confirm(ctx, req.SKU, req.Quantity); err != nil {
		if errors.Is(err, store.ErrInsufficient) {
			http.Error(w, "insufficient stock", http.StatusConflict)
			return
		}
		http.Error(w, fmt.Sprintf("confirm failed: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "inventory-api",
		"version": "1.0.0",
	})
}

func (h *Handler) Ready(w http.ResponseWriter, _ *http.Request) {
	if err := h.store.DB().Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
