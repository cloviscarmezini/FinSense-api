package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"finsense-api/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(cfg *config.Config) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.DBMaxOpenConns)
	poolConfig.MinConns = int32(cfg.DBMaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.DBConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		log.Printf("[DB WARNING] Failed to ping database on startup: %v. Database might still be initializing.", err)
	} else {
		log.Println("[DB] Successfully connected to PostgreSQL.")
		// Run idempotent schema migrations
		if err := runMigrations(ctx, pool); err != nil {
			log.Printf("[DB WARNING] Failed to run schema migrations: %v", err)
		}
	}

	return &DB{Pool: pool}, nil
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrations := []string{
		`ALTER TABLE category_quota_rules ADD COLUMN IF NOT EXISTS quota_type VARCHAR(50) NOT NULL DEFAULT 'expense';`,
		`ALTER TABLE category_quota_rules ADD COLUMN IF NOT EXISTS linked_investment_id UUID REFERENCES investment_items(id) ON DELETE SET NULL;`,
		`ALTER TABLE category_quota_rules ALTER COLUMN category_id DROP NOT NULL;`,
	}

	for _, stmt := range migrations {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			log.Printf("[DB MIGRATION NOTE] Statement execution note: %v", err)
		}
	}
	return nil
}

func (db *DB) Ping(ctx context.Context) error {
	if db.Pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	return db.Pool.Ping(ctx)
}

func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}
