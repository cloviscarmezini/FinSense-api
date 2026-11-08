package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"finsense-api/internal/config"
	"finsense-api/internal/database"
	"finsense-api/internal/models"
	"finsense-api/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserAlreadyExists = errors.New("user already exists with this email")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound      = errors.New("user not found")
)

type AuthService struct {
	db  *database.DB
	cfg *config.Config
}

func NewAuthService(db *database.DB, cfg *config.Config) *AuthService {
	return &AuthService{db: db, cfg: cfg}
}

func (s *AuthService) Register(ctx context.Context, req models.RegisterRequest) (*models.AuthResponse, error) {
	// Check if user already exists
	var existingID uuid.UUID
	err := s.db.Pool.QueryRow(ctx, "SELECT id FROM users WHERE email = $1 AND deleted_at IS NULL", req.Email).Scan(&existingID)
	if err == nil {
		return nil, ErrUserAlreadyExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	userID := uuid.New()
	now := time.Now().UTC()

	_, err = s.db.Pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, full_name, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, true, $5, $5)
	`, userID, req.Email, string(hashedPassword), req.FullName, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	// Generate tokens
	accessToken, refreshToken, err := utils.GenerateTokens(
		userID,
		req.Email,
		s.cfg.JWTSecret,
		s.cfg.JWTAccessExpirationMinutes,
		s.cfg.JWTRefreshExpirationDays,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	return &models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.cfg.JWTAccessExpirationMinutes * 60,
		User: models.UserSummary{
			ID:       userID,
			Email:    req.Email,
			FullName: req.FullName,
		},
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req models.LoginRequest) (*models.AuthResponse, error) {
	var user models.User
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, is_active
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`, req.Email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.FullName, &user.IsActive)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	} else if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	if !user.IsActive {
		return nil, errors.New("user account is inactive")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	accessToken, refreshToken, err := utils.GenerateTokens(
		user.ID,
		user.Email,
		s.cfg.JWTSecret,
		s.cfg.JWTAccessExpirationMinutes,
		s.cfg.JWTRefreshExpirationDays,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	return &models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.cfg.JWTAccessExpirationMinutes * 60,
		User: models.UserSummary{
			ID:       user.ID,
			Email:    user.Email,
			FullName: user.FullName,
		},
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenString string) (*models.AuthResponse, error) {
	claims, err := utils.ValidateToken(refreshTokenString, s.cfg.JWTSecret, "refresh")
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	var user models.User
	err = s.db.Pool.QueryRow(ctx, `
		SELECT id, email, full_name, is_active
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, claims.UserID).Scan(&user.ID, &user.Email, &user.FullName, &user.IsActive)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	if !user.IsActive {
		return nil, errors.New("user account is inactive")
	}

	newAccessToken, newRefreshToken, err := utils.GenerateTokens(
		user.ID,
		user.Email,
		s.cfg.JWTSecret,
		s.cfg.JWTAccessExpirationMinutes,
		s.cfg.JWTRefreshExpirationDays,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	return &models.AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.cfg.JWTAccessExpirationMinutes * 60,
		User: models.UserSummary{
			ID:       user.ID,
			Email:    user.Email,
			FullName: user.FullName,
		},
	}, nil
}

func (s *AuthService) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.UserMeResponse, error) {
	var user models.UserMeResponse
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, email, full_name, is_active, created_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(&user.ID, &user.Email, &user.FullName, &user.IsActive, &user.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return &user, nil
}
