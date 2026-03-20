package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/anti-ai-proxy/proxy/internal/config"
)

// DB wraps a pgxpool.Pool.
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a new database connection pool.
func New(cfg *config.Config) (*DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBSSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	poolConfig.MaxConns = 50
	poolConfig.MinConns = 5

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info().Msg("Connected to PostgreSQL")
	return &DB{Pool: pool}, nil
}

// RunMigrations executes SQL migration files in order.
func (db *DB) RunMigrations(migrationsDir string) error {
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	var sqlFiles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".sql") {
			sqlFiles = append(sqlFiles, f.Name())
		}
	}
	sort.Strings(sqlFiles)

	for _, fname := range sqlFiles {
		path := filepath.Join(migrationsDir, fname)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", fname, err)
		}

		_, err = db.Pool.Exec(context.Background(), string(content))
		if err != nil {
			return fmt.Errorf("execute migration %s: %w", fname, err)
		}
		log.Info().Str("file", fname).Msg("Migration applied")
	}

	return nil
}

// Close closes the connection pool.
func (db *DB) Close() {
	db.Pool.Close()
}
