package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"chipmov/internal/config"
	"chipmov/internal/domain"
	"chipmov/internal/storage"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID int64           `json:"uid"`
	Email  string          `json:"email"`
	Role   domain.UserRole `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	cfg   config.Config
	store *storage.Store
}

type TokenPair struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresAt    time.Time   `json:"expires_at"`
	User         domain.User `json:"user"`
}

func NewService(cfg config.Config, store *storage.Store) *Service {
	return &Service{cfg: cfg, store: store}
}

func (s *Service) BootstrapAdmin(ctx context.Context) error {
	hash, err := HashPassword(s.cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	return s.store.UpsertBootstrapAdmin(ctx, s.cfg.BootstrapAdminName, s.cfg.BootstrapAdminEmail, hash)
}

func (s *Service) Login(ctx context.Context, email, password string) (TokenPair, error) {
	user, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(strings.ToLower(email)))
	if err != nil {
		return TokenPair{}, errors.New("invalid credentials")
	}
	if !user.Active {
		return TokenPair{}, errors.New("user is inactive")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return TokenPair{}, errors.New("invalid credentials")
	}
	if err := s.store.MarkUserLogin(ctx, user.ID); err != nil {
		return TokenPair{}, err
	}
	return s.issueTokenPair(ctx, user)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	hash := HashToken(refreshToken)
	stored, err := s.store.GetRefreshToken(ctx, hash)
	if err != nil {
		return TokenPair{}, errors.New("invalid refresh token")
	}
	if stored.RevokedAt != nil || time.Now().After(stored.ExpiresAt) {
		return TokenPair{}, errors.New("refresh token expired or revoked")
	}
	user, err := s.store.GetUserByID(ctx, stored.UserID)
	if err != nil {
		return TokenPair{}, err
	}
	if !user.Active {
		return TokenPair{}, errors.New("user is inactive")
	}
	if err := s.store.RevokeRefreshToken(ctx, hash); err != nil {
		return TokenPair{}, err
	}
	return s.issueTokenPair(ctx, user)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	return s.store.RevokeRefreshToken(ctx, HashToken(refreshToken))
}

func (s *Service) ValidateAccessToken(tokenValue string) (*Claims, error) {
	tokenValue = strings.TrimSpace(strings.TrimPrefix(tokenValue, "Bearer "))
	if tokenValue == "" {
		return nil, errors.New("missing token")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenValue, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (s *Service) issueTokenPair(ctx context.Context, user domain.User) (TokenPair, error) {
	expiresAt := time.Now().Add(s.cfg.AccessTokenTTL)
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", user.ID),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := randomToken()
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.store.CreateRefreshToken(ctx, user.ID, HashToken(refresh), time.Now().Add(s.cfg.RefreshTokenTTL)); err != nil {
		return TokenPair{}, err
	}
	user.PasswordHash = ""
	return TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresAt: expiresAt, User: user}, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HasPermission(role domain.UserRole, permission string) bool {
	matrix := map[domain.UserRole]map[string]bool{
		domain.RoleAdmin: {
			"*": true,
		},
		domain.RoleSupervisor: {
			"approvals:write": true,
			"reports:read":    true,
			"users:read":      true,
			"iccids:read":     true,
			"operations:read": true,
			"audit:read":      true,
			"settings:read":   true,
		},
		domain.RoleOperator: {
			"recharge:write":  true,
			"iccids:read":     true,
			"operations:read": true,
			"approvals:read":  true,
		},
		domain.RoleViewer: {
			"iccids:read":     true,
			"operations:read": true,
			"reports:read":    true,
			"approvals:read":  true,
			"settings:read":   true,
		},
	}
	perms := matrix[role]
	return perms["*"] || perms[permission]
}
