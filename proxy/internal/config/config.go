package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration.
type Config struct {
	ProxyPort  string
	APIPort    string
	ProxyHost  string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	RedisHost     string
	RedisPort     string
	RedisPassword string

	JWTSecret string
	JWTExpiry time.Duration

	RateLimitRequests int
	RateLimitWindow   int

	SessionTimeout int

	SuspicionThreshold int
	ScoreAIRequest     int
	ScoreProxyDisconnect int
	ScoreMultiBlocked  int

	LogLevel  string
	LogFormat string
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		ProxyPort:  getEnv("PROXY_PORT", "8080"),
		APIPort:    getEnv("API_PORT", "8081"),
		ProxyHost:  getEnv("PROXY_HOST", "0.0.0.0"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "antiproxy"),
		DBPassword: getEnv("DB_PASSWORD", "antiproxy_secret_change_me"),
		DBName:     getEnv("DB_NAME", "antiproxy"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		JWTSecret: getEnv("JWT_SECRET", "change-me-to-a-secure-random-string-at-least-32-chars"),
		JWTExpiry: parseDuration(getEnv("JWT_EXPIRY", "24h")),

		RateLimitRequests: getEnvInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitWindow:   getEnvInt("RATE_LIMIT_WINDOW", 60),

		SessionTimeout: getEnvInt("SESSION_TIMEOUT", 60),

		SuspicionThreshold:  getEnvInt("SUSPICION_THRESHOLD", 50),
		ScoreAIRequest:      getEnvInt("SCORE_AI_REQUEST", 10),
		ScoreProxyDisconnect: getEnvInt("SCORE_PROXY_DISCONNECT", 20),
		ScoreMultiBlocked:   getEnvInt("SCORE_MULTI_BLOCKED", 5),

		LogLevel:  getEnv("LOG_LEVEL", "info"),
		LogFormat: getEnv("LOG_FORMAT", "json"),
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}
