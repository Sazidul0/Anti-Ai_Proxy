package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/anti-ai-proxy/proxy/internal/anticheat"
	"github.com/anti-ai-proxy/proxy/internal/auth"
	"github.com/anti-ai-proxy/proxy/internal/filter"
	"github.com/anti-ai-proxy/proxy/internal/middleware"
	"github.com/anti-ai-proxy/proxy/internal/proxy"
	"github.com/anti-ai-proxy/proxy/internal/session"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server is the REST API server for the admin dashboard and CTFd integration.
type Server struct {
	pool     *pgxpool.Pool
	auth     *auth.Auth
	sessions *session.Manager
	filter   *filter.Engine
	scorer   *anticheat.Scorer
	proxy    *proxy.Proxy
	upgrader websocket.Upgrader
	apiSecret string
}

// NewServer creates a new API server.
func NewServer(pool *pgxpool.Pool, a *auth.Auth, sess *session.Manager, f *filter.Engine, scorer *anticheat.Scorer, p *proxy.Proxy, apiSecret string) *Server {
	return &Server{
		pool:     pool,
		auth:     a,
		sessions: sess,
		filter:   f,
		scorer:   scorer,
		proxy:    p,
		apiSecret: apiSecret,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Router returns the configured HTTP router.
func (s *Server) Router(rateLimiter *middleware.RateLimiter) http.Handler {
	r := mux.NewRouter()

	// Apply global middleware
	r.Use(middleware.CORS)

	// Public routes
	r.HandleFunc("/api/health", s.healthCheck).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/auth/login", s.login).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/auth/register", s.register).Methods("POST", "OPTIONS")

	// Proxy session routes (for proxy users)
	r.HandleFunc("/api/proxy/connect", s.proxyConnect).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/proxy/heartbeat", s.proxyHeartbeat).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/proxy/disconnect", s.proxyDisconnect).Methods("POST", "OPTIONS")

	// CTFd integration routes (secured with API secret)
	r.HandleFunc("/api/sessions/{userId}/active", s.checkUserActive).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/flag-submissions", s.recordFlagSubmission).Methods("POST", "OPTIONS")

	// Admin routes (JWT protected)
	admin := r.PathPrefix("/api").Subrouter()
	admin.Use(s.auth.Middleware)

	admin.HandleFunc("/users", s.getUsers).Methods("GET")
	admin.HandleFunc("/sessions", s.getSessions).Methods("GET")
	admin.HandleFunc("/blocked-requests", s.getBlockedRequests).Methods("GET")
	admin.HandleFunc("/stats", s.getStats).Methods("GET")
	admin.HandleFunc("/alerts", s.getAlerts).Methods("GET")
	admin.HandleFunc("/filter/domains", s.getFilterDomains).Methods("GET")
	admin.HandleFunc("/filter/domains", s.addFilterDomain).Methods("POST")
	admin.HandleFunc("/filter/domains/{domain}", s.removeFilterDomain).Methods("DELETE")

	// WebSocket for real-time updates
	r.HandleFunc("/api/ws", s.handleWebSocket)

	if rateLimiter != nil {
		return rateLimiter.Middleware(r)
	}
	return r
}

// ── Health ──

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	stats := s.proxy.GetStats()
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"service": "anti-ai-proxy",
		"stats":   stats,
	})
}

// ── Auth ──

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	token, err := s.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.Username) < 3 || len(req.Password) < 6 {
		respondError(w, http.StatusBadRequest, "Username must be 3+ chars, password 6+ chars")
		return
	}

	id, token, err := s.auth.RegisterUser(r.Context(), req.Username, req.Password, "user")
	if err != nil {
		respondError(w, http.StatusConflict, "Username already exists")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"user_id": id,
		"token":   token,
	})
}

// ── Proxy Session ──

func (s *Server) proxyConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	claims, err := s.auth.ValidateToken(req.Token)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "Invalid token")
		return
	}

	ip := extractIP(r.RemoteAddr)
	sess, err := s.sessions.Create(r.Context(), claims.UserID, ip, req.Fingerprint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"session_token": sess.SessionToken,
		"session_id":    sess.ID,
	})
}

func (s *Server) proxyHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := s.sessions.Heartbeat(r.Context(), req.SessionToken); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid session")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) proxyDisconnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := s.sessions.Deactivate(r.Context(), req.SessionToken); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid session")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// ── CTFd Integration ──

func (s *Server) checkUserActive(w http.ResponseWriter, r *http.Request) {
	// Verify API secret
	secret := r.Header.Get("X-API-Secret")
	if secret != s.apiSecret {
		respondError(w, http.StatusUnauthorized, "Invalid API secret")
		return
	}

	vars := mux.Vars(r)
	userID, err := strconv.Atoi(vars["userId"])
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	active, sessionToken, err := s.sessions.IsUserActive(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to check session")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":       userID,
		"active":        active,
		"session_token": sessionToken,
	})
}

func (s *Server) recordFlagSubmission(w http.ResponseWriter, r *http.Request) {
	secret := r.Header.Get("X-API-Secret")
	if secret != s.apiSecret {
		respondError(w, http.StatusUnauthorized, "Invalid API secret")
		return
	}

	var req struct {
		UserID      int  `json:"user_id"`
		SessionID   int  `json:"session_id"`
		ChallengeID int  `json:"challenge_id"`
		ProxyConnected bool `json:"proxy_connected"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO flag_submissions (user_id, session_id, challenge_id, proxy_connected)
		 VALUES ($1, $2, $3, $4)`,
		req.UserID, nullIfZero(req.SessionID), req.ChallengeID, req.ProxyConnected,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to record submission")
		return
	}

	// If submitted without proxy, record anti-cheat event
	if !req.ProxyConnected && req.UserID > 0 {
		s.scorer.RecordEvent(r.Context(), req.UserID, anticheat.EventProxyDisconnect, map[string]interface{}{
			"challenge_id": req.ChallengeID,
		})
	}

	respondJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
}

// ── Admin: Users ──

func (s *Server) getUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(),
		`SELECT u.id, u.username, u.role, u.suspicion_score, u.is_suspicious, u.created_at,
		        (SELECT COUNT(*) FROM sessions WHERE user_id = u.id AND is_active = TRUE) as active_sessions,
		        (SELECT COUNT(*) FROM request_logs WHERE user_id = u.id) as total_requests,
		        (SELECT COUNT(*) FROM request_logs WHERE user_id = u.id AND status = 'blocked') as blocked_requests
		 FROM users u ORDER BY u.suspicion_score DESC, u.created_at DESC`,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to query users")
		return
	}
	defer rows.Close()

	type User struct {
		ID              int       `json:"id"`
		Username        string    `json:"username"`
		Role            string    `json:"role"`
		SuspicionScore  int       `json:"suspicion_score"`
		IsSuspicious    bool      `json:"is_suspicious"`
		CreatedAt       time.Time `json:"created_at"`
		ActiveSessions  int       `json:"active_sessions"`
		TotalRequests   int       `json:"total_requests"`
		BlockedRequests int       `json:"blocked_requests"`
	}

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.SuspicionScore, &u.IsSuspicious,
			&u.CreatedAt, &u.ActiveSessions, &u.TotalRequests, &u.BlockedRequests); err == nil {
			users = append(users, u)
		}
	}

	if users == nil {
		users = []User{}
	}
	respondJSON(w, http.StatusOK, users)
}

// ── Admin: Sessions ──

func (s *Server) getSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.sessions.GetActiveSessions(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to query sessions")
		return
	}
	if sessions == nil {
		sessions = []session.Session{}
	}
	respondJSON(w, http.StatusOK, sessions)
}

// ── Admin: Blocked Requests ──

func (s *Server) getBlockedRequests(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	rows, err := s.pool.Query(r.Context(),
		`SELECT rl.id, rl.user_id, COALESCE(u.username, ''), rl.ip_address, rl.domain, rl.url_path,
		        rl.method, rl.status, rl.block_reason, rl.timestamp
		 FROM request_logs rl
		 LEFT JOIN users u ON rl.user_id = u.id
		 WHERE rl.status = 'blocked'
		 ORDER BY rl.timestamp DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to query blocked requests")
		return
	}
	defer rows.Close()

	type BlockedRequest struct {
		ID          int       `json:"id"`
		UserID      *int      `json:"user_id"`
		Username    string    `json:"username"`
		IPAddress   string    `json:"ip_address"`
		Domain      string    `json:"domain"`
		URLPath     string    `json:"url_path"`
		Method      string    `json:"method"`
		Status      string    `json:"status"`
		BlockReason string    `json:"block_reason"`
		Timestamp   time.Time `json:"timestamp"`
	}

	var results []BlockedRequest
	for rows.Next() {
		var br BlockedRequest
		if err := rows.Scan(&br.ID, &br.UserID, &br.Username, &br.IPAddress, &br.Domain,
			&br.URLPath, &br.Method, &br.Status, &br.BlockReason, &br.Timestamp); err == nil {
			results = append(results, br)
		}
	}
	if results == nil {
		results = []BlockedRequest{}
	}
	respondJSON(w, http.StatusOK, results)
}

// ── Admin: Stats ──

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	proxyStats := s.proxy.GetStats()

	// Get DB stats
	var totalUsers, suspiciousUsers, totalSessions, activeSessions int
	var totalRequests, blockedRequests int

	s.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM users").Scan(&totalUsers)
	s.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM users WHERE is_suspicious = TRUE").Scan(&suspiciousUsers)
	s.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM sessions").Scan(&totalSessions)
	s.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM sessions WHERE is_active = TRUE").Scan(&activeSessions)
	s.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM request_logs").Scan(&totalRequests)
	s.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM request_logs WHERE status = 'blocked'").Scan(&blockedRequests)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"proxy":             proxyStats,
		"filter":            s.filter.Stats(),
		"total_users":       totalUsers,
		"suspicious_users":  suspiciousUsers,
		"total_sessions":    totalSessions,
		"active_sessions":   activeSessions,
		"total_requests":    totalRequests,
		"blocked_requests":  blockedRequests,
	})
}

// ── Admin: Alerts ──

func (s *Server) getAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := s.scorer.GetAlerts(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to query alerts")
		return
	}
	if alerts == nil {
		alerts = []anticheat.Alert{}
	}
	respondJSON(w, http.StatusOK, alerts)
}

// ── Admin: Filter Management ──

func (s *Server) getFilterDomains(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"domains": s.filter.GetBlockedDomains(),
		"stats":   s.filter.Stats(),
	})
}

func (s *Server) addFilterDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Domain == "" {
		respondError(w, http.StatusBadRequest, "Invalid domain")
		return
	}

	s.filter.AddDomain(req.Domain)
	respondJSON(w, http.StatusCreated, map[string]string{"status": "added", "domain": req.Domain})
}

func (s *Server) removeFilterDomain(w http.ResponseWriter, r *http.Request) {
	domain := mux.Vars(r)["domain"]
	s.filter.RemoveDomain(domain)
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed", "domain": domain})
}

// ── WebSocket ──

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket upgrade failed")
		return
	}
	defer conn.Close()

	log.Info().Msg("WebSocket client connected")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := s.proxy.GetStats()
		sessions, _ := s.sessions.GetActiveSessions(r.Context())

		data := map[string]interface{}{
			"type":            "update",
			"stats":           stats,
			"active_sessions": len(sessions),
			"timestamp":       time.Now(),
		}

		if err := conn.WriteJSON(data); err != nil {
			log.Debug().Err(err).Msg("WebSocket write failed, closing")
			break
		}
	}
}

// ── Helpers ──

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

func extractIP(addr string) string {
	if idx := len(addr) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if addr[i] == ':' {
				return addr[:i]
			}
		}
	}
	return addr
}

func nullIfZero(v int) interface{} {
	if v == 0 {
		return nil
	}
	return v
}
