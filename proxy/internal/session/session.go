package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// Session represents a user's proxy session.
type Session struct {
	ID               int       `json:"id"`
	UserID           int       `json:"user_id"`
	Username         string    `json:"username,omitempty"`
	SessionToken     string    `json:"session_token"`
	IPAddress        string    `json:"ip_address"`
	DeviceFingerprint string  `json:"device_fingerprint,omitempty"`
	IsActive         bool      `json:"is_active"`
	ConnectionStart  time.Time `json:"connection_start"`
	LastActivity     time.Time `json:"last_activity"`
	ConnectionEnd    *time.Time `json:"connection_end,omitempty"`
}

// Manager handles session lifecycle.
type Manager struct {
	pool    *pgxpool.Pool
	redis   *redis.Client
	timeout time.Duration
}

// NewManager creates a new session manager.
func NewManager(pool *pgxpool.Pool, rdb *redis.Client, timeoutSec int) *Manager {
	m := &Manager{
		pool:    pool,
		redis:   rdb,
		timeout: time.Duration(timeoutSec) * time.Second,
	}
	go m.cleanupLoop()
	return m
}

// Create creates a new session for a user.
func (m *Manager) Create(ctx context.Context, userID int, ip, fingerprint string) (*Session, error) {
	token := uuid.New().String()
	now := time.Now()

	sess := &Session{
		UserID:           userID,
		SessionToken:     token,
		IPAddress:        ip,
		DeviceFingerprint: fingerprint,
		IsActive:         true,
		ConnectionStart:  now,
		LastActivity:     now,
	}

	err := m.pool.QueryRow(ctx,
		`INSERT INTO sessions (user_id, session_token, ip_address, device_fingerprint, is_active, connection_start, last_activity)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		userID, token, ip, fingerprint, true, now, now,
	).Scan(&sess.ID)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// Cache in Redis
	data, _ := json.Marshal(sess)
	m.redis.Set(ctx, "session:"+token, data, m.timeout)
	m.redis.Set(ctx, fmt.Sprintf("user_session:%d", userID), token, m.timeout)

	log.Info().Int("user_id", userID).Str("ip", ip).Msg("Session created")
	return sess, nil
}

// Validate checks if a session token is valid and active.
func (m *Manager) Validate(ctx context.Context, token string) (*Session, error) {
	// Try Redis first
	data, err := m.redis.Get(ctx, "session:"+token).Result()
	if err == nil {
		var sess Session
		if json.Unmarshal([]byte(data), &sess) == nil && sess.IsActive {
			return &sess, nil
		}
	}

	// Fallback to DB
	sess := &Session{}
	err = m.pool.QueryRow(ctx,
		`SELECT id, user_id, session_token, ip_address, device_fingerprint, is_active, connection_start, last_activity
		 FROM sessions WHERE session_token = $1 AND is_active = TRUE`,
		token,
	).Scan(&sess.ID, &sess.UserID, &sess.SessionToken, &sess.IPAddress,
		&sess.DeviceFingerprint, &sess.IsActive, &sess.ConnectionStart, &sess.LastActivity)
	if err != nil {
		return nil, fmt.Errorf("session not found or inactive")
	}

	// Re-cache
	sessData, _ := json.Marshal(sess)
	m.redis.Set(ctx, "session:"+token, sessData, m.timeout)

	return sess, nil
}

// Heartbeat updates the last activity timestamp.
func (m *Manager) Heartbeat(ctx context.Context, token string) error {
	now := time.Now()

	// Update Redis
	data, err := m.redis.Get(ctx, "session:"+token).Result()
	if err == nil {
		var sess Session
		if json.Unmarshal([]byte(data), &sess) == nil {
			sess.LastActivity = now
			newData, _ := json.Marshal(sess)
			m.redis.Set(ctx, "session:"+token, newData, m.timeout)
			m.redis.Set(ctx, fmt.Sprintf("user_session:%d", sess.UserID), token, m.timeout)
		}
	}

	// Update DB async
	go func() {
		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.pool.Exec(dbCtx,
			"UPDATE sessions SET last_activity = $1 WHERE session_token = $2 AND is_active = TRUE",
			now, token,
		)
	}()

	return nil
}

// Deactivate marks a session as inactive.
func (m *Manager) Deactivate(ctx context.Context, token string) error {
	now := time.Now()

	m.redis.Del(ctx, "session:"+token)

	_, err := m.pool.Exec(ctx,
		"UPDATE sessions SET is_active = FALSE, connection_end = $1 WHERE session_token = $2",
		now, token,
	)
	if err != nil {
		return fmt.Errorf("deactivate session: %w", err)
	}

	log.Info().Str("token", token[:8]+"...").Msg("Session deactivated")
	return nil
}

// IsUserActive checks if a user has an active session.
func (m *Manager) IsUserActive(ctx context.Context, userID int) (bool, string, error) {
	token, err := m.redis.Get(ctx, fmt.Sprintf("user_session:%d", userID)).Result()
	if err == nil {
		sess, err := m.Validate(ctx, token)
		if err == nil && sess.IsActive {
			return true, token, nil
		}
	}

	// Fallback to DB
	var sessionToken string
	err = m.pool.QueryRow(ctx,
		"SELECT session_token FROM sessions WHERE user_id = $1 AND is_active = TRUE ORDER BY last_activity DESC LIMIT 1",
		userID,
	).Scan(&sessionToken)
	if err != nil {
		return false, "", nil
	}

	return true, sessionToken, nil
}

// GetActiveSessions returns all active sessions.
func (m *Manager) GetActiveSessions(ctx context.Context) ([]Session, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT s.id, s.user_id, u.username, s.session_token, s.ip_address, s.device_fingerprint,
		        s.is_active, s.connection_start, s.last_activity
		 FROM sessions s
		 JOIN users u ON s.user_id = u.id
		 WHERE s.is_active = TRUE
		 ORDER BY s.last_activity DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		err := rows.Scan(&s.ID, &s.UserID, &s.Username, &s.SessionToken, &s.IPAddress,
			&s.DeviceFingerprint, &s.IsActive, &s.ConnectionStart, &s.LastActivity)
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// GetSessionsByUser returns sessions for a specific user.
func (m *Manager) GetSessionsByUser(ctx context.Context, userID int) ([]Session, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id, user_id, session_token, ip_address, device_fingerprint,
		        is_active, connection_start, last_activity, connection_end
		 FROM sessions WHERE user_id = $1 ORDER BY connection_start DESC LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		err := rows.Scan(&s.ID, &s.UserID, &s.SessionToken, &s.IPAddress,
			&s.DeviceFingerprint, &s.IsActive, &s.ConnectionStart, &s.LastActivity, &s.ConnectionEnd)
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// cleanupLoop periodically marks stale sessions as inactive.
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cutoff := time.Now().Add(-m.timeout)

		result, err := m.pool.Exec(ctx,
			"UPDATE sessions SET is_active = FALSE, connection_end = NOW() WHERE is_active = TRUE AND last_activity < $1",
			cutoff,
		)
		if err != nil {
			log.Error().Err(err).Msg("Session cleanup failed")
		} else if result.RowsAffected() > 0 {
			log.Info().Int64("count", result.RowsAffected()).Msg("Stale sessions cleaned up")
		}
		cancel()
	}
}
