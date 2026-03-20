package filter

import (
	"testing"
)

func TestExactDomainBlocking(t *testing.T) {
	e := New()

	tests := []struct {
		domain  string
		blocked bool
		desc    string
	}{
		{"openai.com", true, "exact match openai.com"},
		{"chat.openai.com", true, "exact match chat.openai.com"},
		{"api.openai.com", true, "exact match api.openai.com"},
		{"claude.ai", true, "exact match claude.ai"},
		{"anthropic.com", true, "exact match anthropic.com"},
		{"perplexity.ai", true, "exact match perplexity.ai"},
		{"poe.com", true, "exact match poe.com"},
		{"huggingface.co", true, "exact match huggingface.co"},
		{"replicate.com", true, "exact match replicate.com"},
		{"phind.com", true, "exact match phind.com"},
		{"google.com", false, "google.com should not be blocked"},
		{"github.com", false, "github.com should not be blocked"},
		{"stackoverflow.com", false, "stackoverflow.com should not be blocked"},
		{"example.com", false, "example.com should not be blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := e.Check(tt.domain, "")
			if result.Blocked != tt.blocked {
				t.Errorf("Check(%q) = blocked:%v, want blocked:%v (reason: %s)", tt.domain, result.Blocked, tt.blocked, result.Reason)
			}
		})
	}
}

func TestSubdomainBlocking(t *testing.T) {
	e := New()

	tests := []struct {
		domain  string
		blocked bool
		desc    string
	}{
		{"new.chat.openai.com", true, "subdomain of openai.com"},
		{"api.anthropic.com", true, "api.anthropic.com exact or subdomain"},
		{"cdn.replicate.com", true, "subdomain of replicate.com"},
		{"random.google.com", false, "subdomain of google.com should not be blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := e.Check(tt.domain, "")
			if result.Blocked != tt.blocked {
				t.Errorf("Check(%q) = blocked:%v, want blocked:%v (reason: %s)", tt.domain, result.Blocked, tt.blocked, result.Reason)
			}
		})
	}
}

func TestWildcardTLDBlocking(t *testing.T) {
	e := New()

	tests := []struct {
		domain  string
		blocked bool
		desc    string
	}{
		{"random.ai", true, ".ai TLD should be blocked"},
		{"some-tool.ai", true, ".ai TLD should be blocked"},
		{"notai.com", false, ".com TLD should not be blocked"},
		{"ai.example.com", false, "ai as subdomain should not be blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := e.Check(tt.domain, "")
			if result.Blocked != tt.blocked {
				t.Errorf("Check(%q) = blocked:%v, want blocked:%v (reason: %s)", tt.domain, result.Blocked, tt.blocked, result.Reason)
			}
		})
	}
}

func TestWhitelist(t *testing.T) {
	e := New()
	e.AddWhitelist("legit-site.ai")

	result := e.Check("legit-site.ai", "")
	if result.Blocked {
		t.Errorf("whitelisted domain legit-site.ai should not be blocked, but was: %s", result.Reason)
	}

	// Non-whitelisted .ai should still be blocked
	result = e.Check("other-site.ai", "")
	if !result.Blocked {
		t.Error("non-whitelisted .ai domain should be blocked")
	}
}

func TestAPIPathBlocking(t *testing.T) {
	e := New()

	tests := []struct {
		domain  string
		path    string
		blocked bool
		desc    string
	}{
		{"example.com", "/v1/chat/completions", true, "OpenAI chat completions path"},
		{"example.com", "/v1/completions", true, "OpenAI completions path"},
		{"example.com", "/v1/embeddings", true, "embeddings path"},
		{"example.com", "/generate", true, "generate path"},
		{"example.com", "/v1/messages", true, "messages path"},
		{"example.com", "/api/generate", true, "api generate path"},
		{"example.com", "/api/chat", true, "api chat path"},
		{"example.com", "/v1/users", false, "normal API path should not be blocked"},
		{"example.com", "/index.html", false, "normal page should not be blocked"},
		{"example.com", "", false, "empty path should not be blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := e.Check(tt.domain, tt.path)
			if result.Blocked != tt.blocked {
				t.Errorf("Check(%q, %q) = blocked:%v, want blocked:%v (reason: %s)", tt.domain, tt.path, result.Blocked, tt.blocked, result.Reason)
			}
		})
	}
}

func TestPortStripping(t *testing.T) {
	e := New()

	result := e.Check("openai.com:443", "")
	if !result.Blocked {
		t.Error("openai.com:443 should be blocked after port stripping")
	}

	result = e.Check("google.com:443", "")
	if result.Blocked {
		t.Error("google.com:443 should not be blocked")
	}
}

func TestAddRemoveDomain(t *testing.T) {
	e := New()

	// Initially not blocked
	result := e.Check("newblock.com", "")
	if result.Blocked {
		t.Error("newblock.com should not be initially blocked")
	}

	// Add and verify blocked
	e.AddDomain("newblock.com")
	result = e.Check("newblock.com", "")
	if !result.Blocked {
		t.Error("newblock.com should be blocked after adding")
	}

	// Remove and verify unblocked
	e.RemoveDomain("newblock.com")
	result = e.Check("newblock.com", "")
	if result.Blocked {
		t.Error("newblock.com should not be blocked after removing")
	}
}

func TestCaseInsensitive(t *testing.T) {
	e := New()

	result := e.Check("OpenAI.COM", "")
	if !result.Blocked {
		t.Error("OpenAI.COM (uppercase) should be blocked")
	}

	result = e.Check("example.com", "/V1/Chat/Completions")
	if !result.Blocked {
		t.Error("path /V1/Chat/Completions should be blocked case-insensitively")
	}
}
