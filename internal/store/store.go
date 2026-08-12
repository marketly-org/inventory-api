// Package store provides the Postgres persistence layer for inventory.
// one query, checks availability in Go, then writes Reserved += qty in
// a second query. This read-then-write is NOT atomic — two concurrent
// requests for the last item both see stock=1, both pass the check,
// both reserve. Stock goes negative.
// The fix is to use a single UPDATE statement with a WHERE clause that
// checks availability atomically:
//	UPDATE products
//	SET reserved = reserved + $1
//	WHERE sku = $2 AND stock - reserved >= $1
// If the UPDATE affects 0 rows, the reservation failed (insufficient
// stock). This is both correct under concurrency and faster (one query
// instead of two).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/marketly-org/inventory-api/internal/models"
)

var (
	ErrNotFound     = errors.New("product not found")
	ErrInsufficient = errors.New("insufficient stock")
)

// Store wraps the database connection.
type Store struct {
	db *sql.DB
}

// New creates a new Store.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &Store{db: db}, nil
}

// InitSchema creates the products table + seeds initial data.
func (s *Store) InitSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS products (
			sku TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			stock INTEGER NOT NULL DEFAULT 0,
			reserved INTEGER NOT NULL DEFAULT 0,
			price_cents INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return fmt.Errorf("create products table: %w", err)
	}

	// Seed products if the table is empty.
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM products").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		seedProducts := []models.Product{
			{SKU: "WIDGET-001", Name: "Widget", Stock: 100, PriceCents: 999},
			{SKU: "GADGET-001", Name: "Gadget", Stock: 50, PriceCents: 1999},
			{SKU: "GIZMO-001", Name: "Gizmo", Stock: 25, PriceCents: 4999},
		}
		for _, p := range seedProducts {
			_, err = s.db.Exec(
				"INSERT INTO products (sku, name, stock, reserved, price_cents) VALUES ($1, $2, $3, 0, $4)",
				p.SKU, p.Name, p.Stock, p.PriceCents,
			)
			if err != nil {
				return fmt.Errorf("seed product %s: %w", p.SKU, err)
			}
		}
	}

	return nil
}

// GetProduct fetches a product by SKU.
func (s *Store) GetProduct(ctx context.Context, sku string) (*models.Product, error) {
	var p models.Product
	err := s.db.QueryRowContext(ctx, `
		SELECT sku, name, stock, reserved, price_cents
		FROM products WHERE sku = $1
	`, sku).Scan(&p.SKU, &p.Name, &p.Stock, &p.Reserved, &p.PriceCents)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	return &p, nil
}

// Reserve increments the Reserved counter for a product.
// availability in Go, then writes the new reserved value in a second
// query. Under concurrent requests, two goroutines can both read
// stock=1, reserved=0, both pass the check, and both write reserved=1.
// The product is now oversold — confirmed later as negative stock.
//   UPDATE products SET reserved = reserved + $1
//   WHERE sku = $2 AND stock - reserved >= $1
// Then check rowsAffected — 0 means insufficient stock.
func (s *Store) Reserve(ctx context.Context, sku string, quantity int) error {
	// READ: fetch current stock + reserved.
	var product models.Product
	err := s.db.QueryRowContext(ctx, `
		SELECT sku, name, stock, reserved, price_cents
		FROM products WHERE sku = $1
		FOR UPDATE
	`, sku).Scan(&product.SKU, &product.Name, &product.Stock, &product.Reserved, &product.PriceCents)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get product for reserve: %w", err)
	}

	// CHECK: availability in Go.
	if product.Available() < quantity {
		return ErrInsufficient
	}

	// WRITE: increment reserved. This is a separate statement — the
	// FOR UPDATE above releases its lock at the end of the transaction,
	// but since we're using a single connection from the pool without
	// an explicit transaction, the lock is released after the SELECT.
	// Another goroutine can read the same stock value before this
	// UPDATE runs.
	_, err = s.db.ExecContext(ctx, `
		UPDATE products SET reserved = reserved + $1 WHERE sku = $2
	`, quantity, sku)
	if err != nil {
		return fmt.Errorf("update reserved: %w", err)
	}

	return nil
}

// Confirm decrements stock + reserved (called when an order is confirmed).
func (s *Store) Confirm(ctx context.Context, sku string, quantity int) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE products
		SET stock = stock - $1, reserved = reserved - $1
		WHERE sku = $2 AND stock >= $1 AND reserved >= $1
	`, quantity, sku)
	if err != nil {
		return fmt.Errorf("confirm stock: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrInsufficient
	}
	return nil
}

// ListProducts returns all products.
func (s *Store) ListProducts(ctx context.Context) ([]models.Product, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sku, name, stock, reserved, price_cents
		FROM products ORDER BY sku
	`)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.SKU, &p.Name, &p.Stock, &p.Reserved, &p.PriceCents); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB (for health checks).
func (s *Store) DB() *sql.DB {
	return s.db
}
