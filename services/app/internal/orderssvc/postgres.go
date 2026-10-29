package orderssvc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// PostgresStore persists Orders in Postgres.
type PostgresStore struct {
	db *sql.DB
}

// OpenPostgres connects, waits for Postgres, and runs the schema migration.
func OpenPostgres(ctx context.Context, dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := db.PingContext(ctx); err == nil {
			break
		}
		if time.Now().After(deadline) {
			db.Close()
			return nil, fmt.Errorf("postgres not ready: %w", err)
		}
		select {
		case <-ctx.Done():
			db.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}

	s := &PostgresStore{db: db}
	if err := s.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Migrate creates the Orders schema.
func (s *PostgresStore) Migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS orders (
	id         TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL,
	total      DOUBLE PRECISION NOT NULL,
	status     TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS order_items (
	order_id   TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
	product_id TEXT NOT NULL,
	quantity   INTEGER NOT NULL
);`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *PostgresStore) Create(ctx context.Context, o Order) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO orders (id, user_id, total, status, created_at) VALUES ($1, $2, $3, $4, $5)`,
		o.ID, o.UserID, o.Total, o.Status, o.CreatedAt,
	); err != nil {
		return err
	}
	for _, item := range o.Items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity) VALUES ($1, $2, $3)`,
			o.ID, item.ProductID, item.Quantity,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *PostgresStore) Get(ctx context.Context, id, userID string) (Order, error) {
	var o Order
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, total, status, created_at FROM orders WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&o.ID, &o.UserID, &o.Total, &o.Status, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT product_id, quantity FROM order_items WHERE order_id = $1 ORDER BY product_id`,
		id,
	)
	if err != nil {
		return Order{}, err
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ProductID, &item.Quantity); err != nil {
			return Order{}, err
		}
		o.Items = append(o.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Order{}, err
	}
	return o, nil
}

func (s *PostgresStore) Reset(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `TRUNCATE order_items, orders`)
	return err
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
