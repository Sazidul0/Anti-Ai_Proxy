package anticheat

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/anti-ai-proxy/proxy/internal/config"
)

// EventType represents a suspicion event type.
type EventType string

const (
	EventAIRequest     EventType = "ai_request_attempt"
	EventProxyDisconnect EventType = "proxy_disconnect_before_flag"
	EventMultiBlocked  EventType = "multiple_blocked_domains"
)

// Event represents a suspicion event.
type Event struct {
	ID        int                    `json:"id"`
	UserID    int                    `json:"user_id"`
	Username  string                 `json:"username,omitempty"`
	EventType EventType              `json:"event_type"`
	ScoreDelta int                   `json:"score_delta"`
	Details   map[string]interface{} `json:"details,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// Alert represents a suspicious user alert.
type Alert struct {
	UserID          int       `json:"user_id"`
	Username        string    `json:"username"`
	SuspicionScore  int       `json:"suspicion_score"`
	IsSuspicious    bool      `json:"is_suspicious"`
	RecentEvents    []Event   `json:"recent_events"`
	LastTriggered   time.Time `json:"last_triggered"`
}

// Scorer handles suspicion scoring.
type Scorer struct {
	pool      *pgxpool.Pool
	redis     *redis.Client
	cfg       *config.Config
	mu        sync.RWMutex
	scores    map[int]int // in-memory cache: user_id -> score
}

// NewScorer creates a new suspicion scorer.
func NewScorer(pool *pgxpool.Pool, rdb *redis.Client, cfg *config.Config) *Scorer {
	return &Scorer{
		pool:   pool,
		redis:  rdb,
		cfg:    cfg,
		scores: make(map[int]int),
	}
}

// RecordEvent records a suspicion event and updates the user's score.
func (s *Scorer) RecordEvent(ctx context.Context, userID int, eventType EventType, details map[string]interface{}) (int, bool, error) {
	var scoreDelta int
	switch eventType {
	case EventAIRequest:
		scoreDelta = s.cfg.ScoreAIRequest
	case EventProxyDisconnect:
		scoreDelta = s.cfg.ScoreProxyDisconnect
	case EventMultiBlocked:
		scoreDelta = s.cfg.ScoreMultiBlocked
	default:
		scoreDelta = 5
	}

	// Record event in DB
	_, err := s.pool.Exec(ctx,
		`INSERT INTO suspicion_events (user_id, event_type, score_delta, details)
		 VALUES ($1, $2, $3, $4)`,
		userID, string(eventType), scoreDelta, details,
	)
	if err != nil {
		log.Error().Err(err).Int("user_id", userID).Msg("Failed to record suspicion event")
	}

	// Update user score
	var newScore int
	var isSuspicious bool
	err = s.pool.QueryRow(ctx,
		`UPDATE users SET
			suspicion_score = suspicion_score + $1,
			is_suspicious = CASE WHEN suspicion_score + $1 >= $2 THEN TRUE ELSE is_suspicious END,
			updated_at = NOW()
		 WHERE id = $3
		 RETURNING suspicion_score, is_suspicious`,
		scoreDelta, s.cfg.SuspicionThreshold, userID,
	).Scan(&newScore, &isSuspicious)
	if err != nil {
		return 0, false, fmt.Errorf("update suspicion score: %w", err)
	}

	// Update cache
	s.mu.Lock()
	s.scores[userID] = newScore
	s.mu.Unlock()

	// Cache in Redis
	s.redis.Set(ctx, fmt.Sprintf("suspicion:%d", userID), newScore, 5*time.Minute)

	if isSuspicious {
		log.Warn().
			Int("user_id", userID).
			Int("score", newScore).
			Str("event", string(eventType)).
			Msg("User marked as SUSPICIOUS")
	}

	return newScore, isSuspicious, nil
}

// GetScore returns the current suspicion score for a user.
func (s *Scorer) GetScore(ctx context.Context, userID int) (int, bool, error) {
	// Try memory cache
	s.mu.RLock()
	if score, ok := s.scores[userID]; ok {
		s.mu.RUnlock()
		return score, score >= s.cfg.SuspicionThreshold, nil
	}
	s.mu.RUnlock()

	// Try DB
	var score int
	var suspicious bool
	err := s.pool.QueryRow(ctx,
		"SELECT suspicion_score, is_suspicious FROM users WHERE id = $1",
		userID,
	).Scan(&score, &suspicious)
	if err != nil {
		return 0, false, err
	}

	s.mu.Lock()
	s.scores[userID] = score
	s.mu.Unlock()

	return score, suspicious, nil
}

// GetAlerts returns all suspicious users with their recent events.
func (s *Scorer) GetAlerts(ctx context.Context) ([]Alert, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT u.id, u.username, u.suspicion_score, u.is_suspicious
		 FROM users u
		 WHERE u.suspicion_score > 0
		 ORDER BY u.suspicion_score DESC
		 LIMIT 100`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.UserID, &a.Username, &a.SuspicionScore, &a.IsSuspicious); err != nil {
			continue
		}

		// Get recent events for this user
		eventRows, err := s.pool.Query(ctx,
			`SELECT id, user_id, event_type, score_delta, details, created_at
			 FROM suspicion_events
			 WHERE user_id = $1
			 ORDER BY created_at DESC LIMIT 10`,
			a.UserID,
		)
		if err == nil {
			for eventRows.Next() {
				var e Event
				if err := eventRows.Scan(&e.ID, &e.UserID, &e.EventType, &e.ScoreDelta, &e.Details, &e.CreatedAt); err == nil {
					e.Username = a.Username
					a.RecentEvents = append(a.RecentEvents, e)
					if a.LastTriggered.IsZero() || e.CreatedAt.After(a.LastTriggered) {
						a.LastTriggered = e.CreatedAt
					}
				}
			}
			eventRows.Close()
		}

		alerts = append(alerts, a)
	}

	return alerts, nil
}

// GetRecentEvents returns the most recent suspicion events.
func (s *Scorer) GetRecentEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT e.id, e.user_id, u.username, e.event_type, e.score_delta, e.details, e.created_at
		 FROM suspicion_events e
		 JOIN users u ON e.user_id = u.id
		 ORDER BY e.created_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.EventType, &e.ScoreDelta, &e.Details, &e.CreatedAt); err == nil {
			events = append(events, e)
		}
	}
	return events, nil
}
