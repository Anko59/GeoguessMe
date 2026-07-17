package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	jwtKey      []byte
	jwtIssuer   = "geoguessme"
	jwtAudience = "geoguessme-web"
	accessTTL   = 15 * time.Minute
)

type Claims struct {
	UserID      string `json:"user_id"`
	AuthVersion int    `json:"auth_version,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	jwt.RegisteredClaims
}

// Init is kept for existing package consumers. Production startup uses
// InitWithSettings so issuer, audience, and lifetime are explicit.
func Init(secret string) {
	if secret == "" && (os.Getenv("GO_TEST") == "1" || strings.Contains(os.Args[0], ".test")) {
		secret = "test_secret_key_for_testing_only"
	}
	jwtKey = []byte(secret)
}

func InitWithSettings(secret, issuer, audience string, ttl time.Duration) {
	Init(secret)
	if issuer != "" {
		jwtIssuer = issuer
	}
	if audience != "" {
		jwtAudience = audience
	}
	if ttl > 0 {
		accessTTL = ttl
	}
}

func CheckPasswordHash(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

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

func GenerateAccessToken(userID string, authVersion int) (string, error) {
	if len(jwtKey) < 32 {
		return "", errors.New("JWT signing secret is not configured")
	}
	now := time.Now()
	claims := &Claims{
		UserID:      userID,
		AuthVersion: authVersion,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Audience:  jwt.ClaimStrings{jwtAudience},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// GenerateToken exists for old unit consumers and intentionally retains the
// historical 24-hour expiry. HTTP handlers use GenerateAccessToken.
func GenerateToken(userID string) (string, error) {
	if len(jwtKey) == 0 {
		Init("")
	}
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtKey)
}

func ValidateAccessToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, keyFunc, jwt.WithIssuer(jwtIssuer), jwt.WithAudience(jwtAudience), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid || claims.TokenType != "access" {
		return nil, errors.New("invalid access token")
	}
	return claims, nil
}

func ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, keyFunc, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}
	return claims, nil
}

func keyFunc(token *jwt.Token) (any, error) {
	if token.Method != jwt.SigningMethodHS256 {
		return nil, errors.New("unexpected signing method")
	}
	return jwtKey, nil
}
