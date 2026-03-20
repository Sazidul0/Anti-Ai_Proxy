package logging

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// RequestLog is an Elasticsearch-compatible log entry.
type RequestLog struct {
	UserID     int       `json:"user_id"`
	SessionID  int       `json:"session_id"`
	IPAddress  string    `json:"ip_address"`
	Domain     string    `json:"domain"`
	URLPath    string    `json:"url_path"`
	Method     string    `json:"method"`
	Status     string    `json:"status"` // "allowed" or "blocked"
	BlockReason string   `json:"block_reason,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// Logger handles structured logging and persistence.
type Logger struct {
	pool   *pgxpool.Pool
	logCh  chan RequestLog
}

// New creates a new logger.
func New(pool *pgxpool.Pool, level string) *Logger {
	// Configure zerolog
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
	zerolog.TimeFieldFormat = time.RFC3339Nano

	log.Logger = zerolog.New(os.Stdout).With().
		Timestamp().
		Str("service", "anti-ai-proxy").
		Logger()

	l := &Logger{
		pool:  pool,
		logCh: make(chan RequestLog, 10000),
	}

	// Start background log writer
	go l.backgroundWriter()

	return l
}

// LogRequest queues a request log for async persistence.
func (l *Logger) LogRequest(entry RequestLog) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Structured log to stdout (Elasticsearch-compatible JSON)
	data, _ := json.Marshal(entry)
	log.Info().RawJSON("request", data).Msg("request_log")

	// Queue for DB persistence
	select {
	case l.logCh <- entry:
	default:
		log.Warn().Msg("Log channel full, dropping entry")
	}
}

func (l *Logger) backgroundWriter() {
	batch := make([]RequestLog, 0, 100)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case entry := <-l.logCh:
			batch = append(batch, entry)
			if len(batch) >= 100 {
				l.flushBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				l.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (l *Logger) flushBatch(batch []RequestLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := l.pool.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to begin log batch transaction")
		return
	}
	defer tx.Rollback(ctx)

	for _, entry := range batch {
		_, err := tx.Exec(ctx,
			`INSERT INTO request_logs (user_id, session_id, ip_address, domain, url_path, method, status, block_reason, timestamp)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			nullIfZero(entry.UserID), nullIfZero(entry.SessionID),
			entry.IPAddress, entry.Domain, entry.URLPath, entry.Method,
			entry.Status, entry.BlockReason, entry.Timestamp,
		)
		if err != nil {
			log.Error().Err(err).Str("domain", entry.Domain).Msg("Failed to insert log entry")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit log batch")
	}
}

func nullIfZero(v int) interface{} {
	if v == 0 {
		return nil
	}
	return v
}
