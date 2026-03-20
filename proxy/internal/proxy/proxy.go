package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/anti-ai-proxy/proxy/internal/anticheat"
	"github.com/anti-ai-proxy/proxy/internal/filter"
	"github.com/anti-ai-proxy/proxy/internal/logging"
	"github.com/anti-ai-proxy/proxy/internal/session"
)

// Stats holds proxy statistics.
type Stats struct {
	ActiveConnections int64 `json:"active_connections"`
	TotalRequests     int64 `json:"total_requests"`
	BlockedRequests   int64 `json:"blocked_requests"`
	AllowedRequests   int64 `json:"allowed_requests"`
}

// Proxy is the core HTTP/HTTPS proxy server.
type Proxy struct {
	filter   *filter.Engine
	sessions *session.Manager
	logger   *logging.Logger
	scorer   *anticheat.Scorer
	stats    Stats
}

// New creates a new proxy server.
func New(f *filter.Engine, sess *session.Manager, logger *logging.Logger, scorer *anticheat.Scorer) *Proxy {
	return &Proxy{
		filter:   f,
		sessions: sess,
		logger:   logger,
		scorer:   scorer,
	}
}

// GetStats returns current proxy stats.
func (p *Proxy) GetStats() Stats {
	return Stats{
		ActiveConnections: atomic.LoadInt64(&p.stats.ActiveConnections),
		TotalRequests:     atomic.LoadInt64(&p.stats.TotalRequests),
		BlockedRequests:   atomic.LoadInt64(&p.stats.BlockedRequests),
		AllowedRequests:   atomic.LoadInt64(&p.stats.AllowedRequests),
	}
}

// ServeHTTP handles incoming proxy requests.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&p.stats.TotalRequests, 1)
	atomic.AddInt64(&p.stats.ActiveConnections, 1)
	defer atomic.AddInt64(&p.stats.ActiveConnections, -1)

	// Extract user session from Proxy-Authorization header
	userID, sessionID := p.extractSession(r)

	if r.Method == http.MethodConnect {
		p.handleConnect(w, r, userID, sessionID)
	} else {
		p.handleHTTP(w, r, userID, sessionID)
	}
}

func (p *Proxy) extractSession(r *http.Request) (int, int) {
	token := r.Header.Get("Proxy-Authorization")
	if token == "" {
		return 0, 0
	}

	// Strip "Basic " or "Bearer " prefix
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	} else if strings.HasPrefix(token, "Basic ") {
		token = strings.TrimPrefix(token, "Basic ")
	}

	sess, err := p.sessions.Validate(r.Context(), token)
	if err != nil {
		return 0, 0
	}

	// Update heartbeat
	p.sessions.Heartbeat(r.Context(), token)

	return sess.UserID, sess.ID
}

// handleConnect handles HTTPS CONNECT tunneling.
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request, userID, sessionID int) {
	host := r.Host
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}

	// Extract domain (without port)
	domain := host
	if idx := strings.LastIndex(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	// Check filter
	result := p.filter.Check(domain, "")
	clientIP := extractIP(r.RemoteAddr)

	if result.Blocked {
		atomic.AddInt64(&p.stats.BlockedRequests, 1)

		p.logger.LogRequest(logging.RequestLog{
			UserID:      userID,
			SessionID:   sessionID,
			IPAddress:   clientIP,
			Domain:      domain,
			Method:      "CONNECT",
			Status:      "blocked",
			BlockReason: result.Reason,
		})

		// Record anti-cheat event
		if userID > 0 {
			p.scorer.RecordEvent(r.Context(), userID, anticheat.EventAIRequest, map[string]interface{}{
				"domain": domain,
				"rule":   result.Rule,
			})
		}

		http.Error(w, "Access Denied: "+result.Reason, http.StatusForbidden)
		return
	}

	atomic.AddInt64(&p.stats.AllowedRequests, 1)

	// Establish tunnel to target
	targetConn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		http.Error(w, "Failed to connect to target", http.StatusBadGateway)
		return
	}

	// Log allowed request
	p.logger.LogRequest(logging.RequestLog{
		UserID:    userID,
		SessionID: sessionID,
		IPAddress: clientIP,
		Domain:    domain,
		Method:    "CONNECT",
		Status:    "allowed",
	})

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		targetConn.Close()
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		targetConn.Close()
		http.Error(w, "Hijack failed", http.StatusInternalServerError)
		return
	}

	// Send 200 Connection Established
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Bidirectional tunnel
	go transfer(targetConn, clientConn)
	go transfer(clientConn, targetConn)
}

// handleHTTP handles plain HTTP proxy requests.
func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request, userID, sessionID int) {
	domain := r.Host
	if idx := strings.LastIndex(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	path := r.URL.Path
	clientIP := extractIP(r.RemoteAddr)

	// Check filter (domain + path)
	result := p.filter.Check(domain, path)

	if result.Blocked {
		atomic.AddInt64(&p.stats.BlockedRequests, 1)

		p.logger.LogRequest(logging.RequestLog{
			UserID:      userID,
			SessionID:   sessionID,
			IPAddress:   clientIP,
			Domain:      domain,
			URLPath:     path,
			Method:      r.Method,
			Status:      "blocked",
			BlockReason: result.Reason,
		})

		if userID > 0 {
			p.scorer.RecordEvent(r.Context(), userID, anticheat.EventAIRequest, map[string]interface{}{
				"domain": domain,
				"path":   path,
				"rule":   result.Rule,
			})
		}

		http.Error(w, "Access Denied: "+result.Reason, http.StatusForbidden)
		return
	}

	atomic.AddInt64(&p.stats.AllowedRequests, 1)

	// Log allowed request
	p.logger.LogRequest(logging.RequestLog{
		UserID:    userID,
		SessionID: sessionID,
		IPAddress: clientIP,
		Domain:    domain,
		URLPath:   path,
		Method:    r.Method,
		Status:    "allowed",
	})

	// Forward the request
	r.RequestURI = ""

	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, vals := range resp.Header {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func transfer(dst io.WriteCloser, src io.ReadCloser) {
	defer dst.Close()
	defer src.Close()
	io.Copy(dst, src)
}

func extractIP(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// ListenAndServe starts the proxy server.
func (p *Proxy) ListenAndServe(addr string) error {
	server := &http.Server{
		Addr:         addr,
		Handler:      p,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Info().Str("addr", addr).Msg("Proxy server starting")
	return server.ListenAndServe()
}

// HealthCheck returns a handler for proxy health checks.
func (p *Proxy) HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		_ = ctx
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"proxy"}`))
	}
}
