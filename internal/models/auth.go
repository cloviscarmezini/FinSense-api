package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID  `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	FullName     string     `json:"full_name"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

type RegisterRequest struct {
	Email      string  `json:"email"`
	Password   string  `json:"password"`
	FullName   string  `json:"full_name"`
	DeviceID   *string `json:"device_id,omitempty"`
	Platform   *string `json:"platform,omitempty"`
	AppVersion *string `json:"app_version,omitempty"`
	DeviceName *string `json:"device_name,omitempty"`
}

type LoginRequest struct {
	Email      string  `json:"email"`
	Password   string  `json:"password"`
	DeviceID   *string `json:"device_id,omitempty"`
	Platform   *string `json:"platform,omitempty"`
	AppVersion *string `json:"app_version,omitempty"`
	DeviceName *string `json:"device_name,omitempty"`
}

type RefreshTokenRequest struct {
	RefreshToken string  `json:"refresh_token"`
	DeviceID     *string `json:"device_id,omitempty"`
	Platform     *string `json:"platform,omitempty"`
	AppVersion   *string `json:"app_version,omitempty"`
	DeviceName   *string `json:"device_name,omitempty"`
}

type DeviceSession struct {
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	DeviceID     string    `json:"device_id"`
	Platform     string    `json:"platform"`
	AppVersion   string    `json:"app_version"`
	DeviceName   *string   `json:"device_name,omitempty"`
	IPAddress    *string   `json:"ip_address,omitempty"`
	LastAccessAt time.Time `json:"last_access_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UserSummary struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	FullName string    `json:"full_name"`
}

type AuthResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    int         `json:"expires_in"` // in seconds
	User         UserSummary `json:"user"`
}

type UserMeResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}
