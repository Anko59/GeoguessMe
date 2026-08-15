package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Service issues and validates access tokens. PR 7 converts the historical
// package-level jwt globals (jwtKey, jwtIssuer, jwtAudience, accessTTL) into an
// explicit per-instance service: every App constructs one from validated
// configuration, so two application instances never share signing state.
// Instances are independent and immutable after construction.
type Service struct {
	key       []byte
	issuer    string
	audience  string
	accessTTL time.Duration
}

// NewService returns an explicit token service bound to the configured signing
// secret and claim profile. An empty issuer or audience keeps the default
// values; a zero ttl keeps the default 15-minute access lifetime.
func NewService(secret, issuer, audience string, ttl time.Duration) *Service {
	service := &Service{key: []byte(secret), issuer: "geoguessme", audience: "geoguessme-web", accessTTL: 15 * time.Minute}
	if issuer != "" {
		service.issuer = issuer
	}
	if audience != "" {
		service.audience = audience
	}
	if ttl > 0 {
		service.accessTTL = ttl
	}
	return service
}

type Claims struct {
	UserID      string `json:"user_id"`
	AuthVersion int    `json:"auth_version,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	jwt.RegisteredClaims
}

// CheckPasswordHash reports whether a bcrypt hash matches the plain password.
// It is a pure helper with no mutable state and stays a package function.
func CheckPasswordHash(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// HashToken one-way digests an opaque token for storage. Pure helper.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// GenerateOpaqueToken returns a cryptographically random URL-safe token.
// Pure helper.
func GenerateOpaqueToken(bytesLength int) (string, error) {
	if bytesLength < 32 {
		bytesLength = 32
	}
	buffer := make([]byte, bytesLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// GenerateAccessToken issues a short-lived access token bound to the user's
// current auth version.
func (s *Service) GenerateAccessToken(userID string, authVersion int) (string, error) {
	if len(s.key) < 32 {
		return "", errors.New("JWT signing secret is not configured")
	}
	now := time.Now()
	claims := &Claims{
		UserID:      userID,
		AuthVersion: authVersion,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Audience:  jwt.ClaimStrings{s.audience},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.key)
}

// GenerateToken exists for old unit consumers and intentionally retains the
// historical 24-hour expiry. HTTP handlers use GenerateAccessToken.
func (s *Service) GenerateToken(userID string) (string, error) {
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
}

func (s *Service) ValidateAccessToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, s.keyFunc, jwt.WithIssuer(s.issuer), jwt.WithAudience(s.audience), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid || claims.TokenType != "access" {
		return nil, errors.New("invalid access token")
	}
	return claims, nil
}

func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, s.keyFunc, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}
	return claims, nil
}

func (s *Service) keyFunc(token *jwt.Token) (any, error) {
	if token.Method != jwt.SigningMethodHS256 {
		return nil, errors.New("unexpected signing method")
	}
	return s.key, nil
}
