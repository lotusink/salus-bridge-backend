package database

import (
	"context"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// NewDBConnector creates and returns a new DBConnector instance with the provided configuration.
// It establishes a PostgreSQL database connection, configures the connection pool settings,
// and verifies the connection with a 5-second timeout ping.
//
// Parameters:
//   - cfg: DBConnectorConfig containing the DSN and connection pool settings.
//
// Returns:
//   - *DBConnector: a pointer to the newly created DBConnector instance.
//   - error: if the connection cannot be established or the ping times out.
func NewDBConnector(cfg DBConnectorConfig) (*DBConnector, error) {
	// Create the connection
	db, err := sqlx.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("error when connecting to database: %w", err)
	}

	// Create the connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifeTime)

	// Testing connection
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeOut)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("error when connecting to database: %w", err)
	}

	return &DBConnector{db: db}, nil
}

// PingContext verifies the database connection is still alive. The caller
// supplies the deadline via ctx.
func (c *DBConnector) PingContext(ctx context.Context) error {
	return c.db.PingContext(ctx)
}
