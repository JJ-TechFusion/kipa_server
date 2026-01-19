package database

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"kipa/internal/config"
)

type PostgresClient struct {
	DB *sqlx.DB
}

func NewPostgresClient(cfg *config.DatabaseConfig) (*PostgresClient, error) {
	db, err := sqlx.Connect("postgres", cfg.GetDSN())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	return &PostgresClient{DB: db}, nil
}

func (c *PostgresClient) Close() error {
	return c.DB.Close()
}

func (c *PostgresClient) Ping(ctx context.Context) error {
	return c.DB.PingContext(ctx)
}

func (c *PostgresClient) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return c.DB.BeginTxx(ctx, nil)
}
