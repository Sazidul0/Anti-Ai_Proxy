package filter

import (
	"strings"
	"sync"
)

// Result represents the outcome of a filter check.
type Result struct {
	Blocked bool
	Reason  string
	Rule    string // which rule triggered the block
}

// Engine is the AI filtering engine.
type Engine struct {
	mu             sync.RWMutex
	blockedDomains map[string]bool
	blockedTLDs    []string
	blockedPaths   []string
	whitelisted    map[string]bool
}

// New creates a new filter engine with default rules.
func New() *Engine {
	e := &Engine{
		blockedDomains: make(map[string]bool),
		whitelisted:    make(map[string]bool),
	}
	e.loadDefaults()
	return e
}

func (e *Engine) loadDefaults() {
	// ── Exact domain blocklist ──
	domains := []string{
		"openai.com",
		"chat.openai.com",
		"api.openai.com",
		"platform.openai.com",
		"claude.ai",
		"anthropic.com",
		"api.anthropic.com",
		"perplexity.ai",
		"poe.com",
		"huggingface.co",
		"api-inference.huggingface.co",
		"replicate.com",
		"api.replicate.com",
		"phind.com",
		"bard.google.com",
		"gemini.google.com",
		"generativelanguage.googleapis.com",
		"aistudio.google.com",
		"copilot.microsoft.com",
		"copilot.github.com",
		"codeium.com",
		"tabnine.com",
		"writesonic.com",
		"jasper.ai",
		"you.com",
		"character.ai",
		"beta.character.ai",
		"deepseek.com",
		"chat.deepseek.com",
		"api.deepseek.com",
		"groq.com",
		"api.groq.com",
		"mistral.ai",
		"api.mistral.ai",
		"cohere.ai",
		"api.cohere.ai",
		"together.ai",
		"api.together.ai",
		"fireworks.ai",
		"api.fireworks.ai",
		"cursor.sh",
		"cursor.com",
		"v0.dev",
		"bolt.new",
	}
	for _, d := range domains {
		e.blockedDomains[d] = true
	}

	// ── Wildcard TLD blocking ──
	e.blockedTLDs = []string{
		".ai",
	}

	// ── API path patterns ──
	e.blockedPaths = []string{
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
		"/v1/images/generations",
		"/v1/audio/transcriptions",
		"/v1/messages",
		"/generate",
		"/api/generate",
		"/api/chat",
		"/chat/completions",
	}

	// ── Whitelisted .ai domains (legitimate non-AI sites) ──
	whitelist := []string{
		"wai.ai",  // example placeholder
	}
	for _, d := range whitelist {
		e.whitelisted[d] = true
	}
}

// Check evaluates a request against all filter rules.
// domain should be lowercase and without port.
// path can be empty for CONNECT-only checks.
func (e *Engine) Check(domain, path string) Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSpace(domain))
	path = strings.ToLower(strings.TrimSpace(path))

	// Strip port if present
	if idx := strings.LastIndex(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	// 1. Exact domain match
	if e.blockedDomains[domain] {
		return Result{Blocked: true, Reason: "Blocked AI domain: " + domain, Rule: "domain_exact"}
	}

	// Also check parent domain (e.g., api.openai.com → openai.com)
	parts := strings.SplitN(domain, ".", 2)
	if len(parts) == 2 {
		parent := parts[1]
		if e.blockedDomains[parent] {
			return Result{Blocked: true, Reason: "Blocked AI domain (subdomain of " + parent + "): " + domain, Rule: "domain_parent"}
		}
	}

	// 2. Wildcard TLD match (skip whitelisted)
	if !e.whitelisted[domain] {
		for _, tld := range e.blockedTLDs {
			if strings.HasSuffix(domain, tld) {
				return Result{Blocked: true, Reason: "Blocked TLD " + tld + ": " + domain, Rule: "tld_wildcard"}
			}
		}
	}

	// 3. API path pattern detection
	if path != "" {
		for _, pattern := range e.blockedPaths {
			if strings.HasPrefix(path, pattern) {
				return Result{Blocked: true, Reason: "Blocked AI API pattern: " + pattern, Rule: "api_pattern"}
			}
		}
	}

	return Result{Blocked: false}
}

// AddDomain adds a domain to the blocklist.
func (e *Engine) AddDomain(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.blockedDomains[strings.ToLower(domain)] = true
}

// RemoveDomain removes a domain from the blocklist.
func (e *Engine) RemoveDomain(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.blockedDomains, strings.ToLower(domain))
}

// AddWhitelist adds a domain to the whitelist.
func (e *Engine) AddWhitelist(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.whitelisted[strings.ToLower(domain)] = true
}

// GetBlockedDomains returns all blocked domains.
func (e *Engine) GetBlockedDomains() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	domains := make([]string, 0, len(e.blockedDomains))
	for d := range e.blockedDomains {
		domains = append(domains, d)
	}
	return domains
}

// Stats returns filter rule counts.
func (e *Engine) Stats() map[string]int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return map[string]int{
		"blocked_domains": len(e.blockedDomains),
		"blocked_tlds":    len(e.blockedTLDs),
		"blocked_paths":   len(e.blockedPaths),
		"whitelisted":     len(e.whitelisted),
	}
}
