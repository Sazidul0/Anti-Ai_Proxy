package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/anti-ai-proxy/proxy/internal/config"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
)

// Claims represents JWT claims.
type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Auth handles JWT authentication.
type Auth struct {
	secret []byte
	expiry time.Duration
	pool   *pgxpool.Pool
}

// New creates a new Auth handler.
func New(cfg *config.Config, pool *pgxpool.Pool) *Auth {
	return &Auth{
		secret: []byte(cfg.JWTSecret),
		expiry: cfg.JWTExpiry,
		pool:   pool,
	}
}

// Login validates credentials and returns a JWT token.
func (a *Auth) Login(ctx context.Context, username, password string) (string, error) {
	var id int
	var hash string
	var role string

	err := a.pool.QueryRow(ctx,
		"SELECT id, password_hash, role FROM users WHERE username = $1",
		username,
	).Scan(&id, &hash, &role)
	if err != nil {
		return "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	return a.generateToken(id, username, role)
}

// RegisterUser creates a new user and returns a JWT token.
func (a *Auth) RegisterUser(ctx context.Context, username, password, role string) (int, string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, "", fmt.Errorf("hash password: %w", err)
	}

	if role == "" {
		role = "user"
	}

	var id int
	err = a.pool.QueryRow(ctx,
		"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, $3) RETURNING id",
		username, string(hash), role,
	).Scan(&id)
	if err != nil {
		return 0, "", fmt.Errorf("insert user: %w", err)
	}

	token, err := a.generateToken(id, username, role)
	if err != nil {
		return 0, "", err
	}

	return id, token, nil
}

func (a *Auth) generateToken(userID int, username, role string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(a.expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "anti-ai-proxy",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.secret)
}

// ValidateToken parses and validates a JWT token.
func (a *Auth) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return a.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// Middleware returns an HTTP middleware that validates JWT tokens.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		claims, err := a.ValidateToken(parts[1])
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminOnly returns middleware that requires admin role.
func (a *Auth) AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil || claims.Role != "admin" {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type contextKey string

const claimsKey contextKey = "claims"

// GetClaims extracts claims from context.
func GetClaims(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}
