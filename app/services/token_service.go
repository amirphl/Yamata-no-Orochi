// Package services provides external service integrations and technical concerns like notifications and tokens
package services

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token service error constants
var (
	ErrTokenExpired = errors.New("token has expired")
	ErrTokenInvalid = errors.New("invalid token")
	ErrTokenRevoked = errors.New("token has been revoked")
)

// TokenService handles JWT token generation and validation
type TokenService interface {
	GenerateTokens(customerID uint) (accessToken, refreshToken string, err error)
	ValidateToken(token string) (*TokenClaims, error)
	RefreshToken(refreshToken string) (newAccessToken, newRefreshToken string, err error)
	RevokeToken(token string) error
	GetTokenClaims(token string) (*TokenClaims, error)
	IsTokenRevoked(token string) bool
}

// TokenClaims represents the claims in a JWT token
type TokenClaims struct {
	CustomerID uint      `json:"customer_id"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	TokenType  string    `json:"token_type"` // "access" or "refresh"
	TokenID    string    `json:"jti"`        // JWT ID for token revocation
}

// TokenServiceImpl implements TokenService
type TokenServiceImpl struct {
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	signingMethod   jwt.SigningMethod
	privateKey      *rsa.PrivateKey
	publicKey       *rsa.PublicKey
	issuer          string
	audience        string
}

// NewTokenService creates a new token service
func NewTokenService(accessTokenTTL, refreshTokenTTL time.Duration, issuer, audience string) (TokenService, error) {
	// Load or generate RSA key pair
	privateKey, publicKey, err := loadOrGenerateRSAKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to load/generate RSA keys: %w", err)
	}

	return &TokenServiceImpl{
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
		signingMethod:   jwt.SigningMethodRS256,
		privateKey:      privateKey,
		publicKey:       publicKey,
		issuer:          issuer,
		audience:        audience,
	}, nil
}

// loadOrGenerateRSAKeys loads existing RSA keys or generates new ones
func loadOrGenerateRSAKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKeyPath := "jwt_private.pem"
	publicKeyPath := "jwt_public.pem"

	// Try to load existing keys
	if privateKeyBytes, err := os.ReadFile(privateKeyPath); err == nil {
		if publicKeyBytes, err := os.ReadFile(publicKeyPath); err == nil {
			// Load private key
			privateKeyBlock, _ := pem.Decode(privateKeyBytes)
			if privateKeyBlock == nil {
				return nil, nil, fmt.Errorf("failed to decode private key")
			}

			privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBlock.Bytes)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
			}

			// Load public key
			publicKeyBlock, _ := pem.Decode(publicKeyBytes)
			if publicKeyBlock == nil {
				return nil, nil, fmt.Errorf("failed to decode public key")
			}

			publicKey, err := x509.ParsePKIXPublicKey(publicKeyBlock.Bytes)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse public key: %w", err)
			}

			rsaPublicKey, ok := publicKey.(*rsa.PublicKey)
			if !ok {
				return nil, nil, fmt.Errorf("public key is not RSA")
			}

			return privateKey, rsaPublicKey, nil
		}
	}

	// Generate new keys
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Save private key
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	if err := os.WriteFile(privateKeyPath, privateKeyPEM, 0600); err != nil {
		return nil, nil, fmt.Errorf("failed to save private key: %w", err)
	}

	// Save public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	if err := os.WriteFile(publicKeyPath, publicKeyPEM, 0644); err != nil {
		return nil, nil, fmt.Errorf("failed to save public key: %w", err)
	}

	return privateKey, &privateKey.PublicKey, nil
}

// GenerateTokens generates access and refresh tokens for a customer
func (s *TokenServiceImpl) GenerateTokens(customerID uint) (accessToken, refreshToken string, err error) {
	now := time.Now()

	// Generate unique token IDs
	accessTokenID, err := generateTokenID()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token ID: %w", err)
	}

	refreshTokenID, err := generateTokenID()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token ID: %w", err)
	}

	// Generate access token
	accessClaims := jwt.MapClaims{
		"customer_id": customerID,
		"token_type":  "access",
		"jti":         accessTokenID,
		"iat":         now.Unix(),
		"exp":         now.Add(s.accessTokenTTL).Unix(),
		"iss":         s.issuer,
		"aud":         s.audience,
	}

	accessToken, err = s.generateToken(accessClaims)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token
	refreshClaims := jwt.MapClaims{
		"customer_id": customerID,
		"token_type":  "refresh",
		"jti":         refreshTokenID,
		"iat":         now.Unix(),
		"exp":         now.Add(s.refreshTokenTTL).Unix(),
		"iss":         s.issuer,
		"aud":         s.audience,
	}

	refreshToken, err = s.generateToken(refreshClaims)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

// ValidateToken validates a JWT token and returns claims
func (s *TokenServiceImpl) ValidateToken(token string) (*TokenClaims, error) {
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return s.publicKey, nil
	})

	if err != nil {
		// Check if the error is due to token expiration
		if strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "exp") {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	if !parsedToken.Valid {
		return nil, ErrTokenInvalid
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrTokenInvalid
	}

	// Extract claims
	customerID, ok := claims["customer_id"].(float64)
	if !ok {
		return nil, ErrTokenInvalid
	}

	tokenType, ok := claims["token_type"].(string)
	if !ok {
		return nil, ErrTokenInvalid
	}

	tokenID, ok := claims["jti"].(string)
	if !ok {
		return nil, ErrTokenInvalid
	}

	issuedAt, ok := claims["iat"].(float64)
	if !ok {
		return nil, ErrTokenInvalid
	}

	expiresAt, ok := claims["exp"].(float64)
	if !ok {
		return nil, ErrTokenInvalid
	}

	// Check if token has expired
	if time.Now().After(time.Unix(int64(expiresAt), 0)) {
		return nil, ErrTokenExpired
	}

	return &TokenClaims{
		CustomerID: uint(customerID),
		TokenType:  tokenType,
		TokenID:    tokenID,
		IssuedAt:   time.Unix(int64(issuedAt), 0),
		ExpiresAt:  time.Unix(int64(expiresAt), 0),
	}, nil
}

// RefreshToken generates new tokens using a refresh token
func (s *TokenServiceImpl) RefreshToken(refreshToken string) (newAccessToken, newRefreshToken string, err error) {
	// Validate refresh token
	claims, err := s.ValidateToken(refreshToken)
	if err != nil {
		return "", "", fmt.Errorf("invalid refresh token: %w", err)
	}

	if claims.TokenType != "refresh" {
		return "", "", fmt.Errorf("token is not a refresh token")
	}

	if time.Now().After(claims.ExpiresAt) {
		return "", "", fmt.Errorf("refresh token has expired")
	}

	// Generate new tokens
	return s.GenerateTokens(claims.CustomerID)
}

// RevokeToken marks a token as revoked (in a real implementation, you'd store this in a database)
func (s *TokenServiceImpl) RevokeToken(token string) error {
	// In a production environment, you would:
	// 1. Validate the token
	// 2. Extract the token ID (jti)
	// 3. Store the token ID in a revocation list (Redis/database)
	// 4. Set an expiration on the revoked token entry

	claims, err := s.ValidateToken(token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// For now, we'll just validate the token
	// In production, you'd add the token ID to a revocation list
	_ = claims

	return nil
}

// GetTokenClaims extracts claims from a token without full validation
func (s *TokenServiceImpl) GetTokenClaims(token string) (*TokenClaims, error) {
	// Parse token without validation (for debugging/logging purposes)
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return s.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Extract claims (same as ValidateToken but without validation checks)
	customerID, ok := claims["customer_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid customer_id claim")
	}

	tokenType, ok := claims["token_type"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid token_type claim")
	}

	tokenID, ok := claims["jti"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid jti claim")
	}

	issuedAt, ok := claims["iat"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid iat claim")
	}

	expiresAt, ok := claims["exp"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid exp claim")
	}

	return &TokenClaims{
		CustomerID: uint(customerID),
		TokenType:  tokenType,
		TokenID:    tokenID,
		IssuedAt:   time.Unix(int64(issuedAt), 0),
		ExpiresAt:  time.Unix(int64(expiresAt), 0),
	}, nil
}

// IsTokenRevoked checks if a token has been revoked
// In a production environment, this would check against a revocation list (Redis/database)
func (s *TokenServiceImpl) IsTokenRevoked(token string) bool {
	// For now, we'll just validate the token
	// In production, you'd check against a revocation list
	_, err := s.ValidateToken(token)
	return err != nil
}

// generateToken creates a signed JWT token
func (s *TokenServiceImpl) generateToken(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(s.signingMethod, claims)
	return token.SignedString(s.privateKey)
}

// generateTokenID generates a unique token ID
func generateTokenID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", bytes), nil
}
