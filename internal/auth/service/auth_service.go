package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"vpsmonitoring-backend/internal/auth/dto"
	"vpsmonitoring-backend/internal/auth/models"
	"vpsmonitoring-backend/internal/auth/repository"
)

var (
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrInactiveAccount      = errors.New("account is inactive; contact an administrator")
	ErrEmailAlreadyExists   = errors.New("email is already registered")
	ErrUserNotFound         = errors.New("user not found")
	ErrCannotSelfDeactivate = errors.New("cannot deactivate your own admin account")
	ErrInvalidToken         = errors.New("invalid or expired token")
)

// UserClaims defines custom claims embedded in the JWT token
type UserClaims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

// AuthService defines business logic operations for authentication & admin management
type AuthService interface {
	Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error)
	Register(ctx context.Context, req dto.RegisterRequest) (*dto.LoginResponse, error)
	GetAdmins(ctx context.Context) ([]dto.UserResponse, error)
	CreateAdmin(ctx context.Context, req dto.CreateAdminRequest) (*dto.UserResponse, error)
	UpdateAdmin(ctx context.Context, targetID int64, req dto.UpdateAdminRequest) (*dto.UserResponse, error)
	UpdateStatus(ctx context.Context, targetID int64, callerID int64, req dto.UpdateStatusRequest) (*dto.UserResponse, error)
	GetProfile(ctx context.Context, userID int64) (*dto.UserResponse, error)
	GenerateToken(user *models.User) (string, error)
	ValidateToken(tokenStr string) (*UserClaims, error)
}

type authService struct {
	userRepo  repository.UserRepository
	jwtSecret string
}

// NewAuthService creates a new instance of AuthService
func NewAuthService(userRepo repository.UserRepository, jwtSecret string) AuthService {
	return &authService{
		userRepo:  userRepo,
		jwtSecret: jwtSecret,
	}
}

func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return nil, errors.New("email and password are required")
	}

	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if user.Status != models.StatusActive {
		return nil, ErrInactiveAccount
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := s.GenerateToken(user)
	if err != nil {
		return nil, errors.New("failed to generate access token")
	}

	return &dto.LoginResponse{
		AccessToken: token,
		Token:       token,
		TokenType:   "Bearer",
		User:        dto.ToUserResponse(user),
	}, nil
}

func (s *authService) Register(ctx context.Context, req dto.RegisterRequest) (*dto.LoginResponse, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Name == "" || req.Email == "" || len(req.Password) < 6 {
		return nil, errors.New("valid name, email, and password (minimum 6 characters) are required")
	}

	exists, err := s.userRepo.EmailExists(ctx, req.Email, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrEmailAlreadyExists
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	newUser := models.User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashed),
		Role:         models.RoleUser, // Strictly USER role
		Status:       models.StatusActive,
	}

	if err := s.userRepo.Create(ctx, &newUser); err != nil {
		return nil, err
	}

	token, _ := s.GenerateToken(&newUser)

	return &dto.LoginResponse{
		AccessToken: token,
		Token:       token,
		TokenType:   "Bearer",
		User:        dto.ToUserResponse(&newUser),
	}, nil
}

func (s *authService) GetAdmins(ctx context.Context) ([]dto.UserResponse, error) {
	users, err := s.userRepo.FindAllByRole(ctx, models.RoleAdmin)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.UserResponse, len(users))
	for i, u := range users {
		responses[i] = dto.ToUserResponse(&u)
	}
	return responses, nil
}

func (s *authService) CreateAdmin(ctx context.Context, req dto.CreateAdminRequest) (*dto.UserResponse, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Name == "" || req.Email == "" || len(req.Password) < 6 {
		return nil, errors.New("valid name, email, and password (minimum 6 characters) are required")
	}

	exists, err := s.userRepo.EmailExists(ctx, req.Email, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrEmailAlreadyExists
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	newAdmin := models.User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashed),
		Role:         models.RoleAdmin, // Strictly ADMIN role
		Status:       models.StatusActive,
	}

	if err := s.userRepo.Create(ctx, &newAdmin); err != nil {
		return nil, err
	}

	res := dto.ToUserResponse(&newAdmin)
	return &res, nil
}

func (s *authService) UpdateAdmin(ctx context.Context, targetID int64, req dto.UpdateAdminRequest) (*dto.UserResponse, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Name == "" || req.Email == "" {
		return nil, errors.New("name and email cannot be empty")
	}

	admin, err := s.userRepo.FindByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if admin == nil || admin.Role != models.RoleAdmin {
		return nil, ErrUserNotFound
	}

	exists, err := s.userRepo.EmailExists(ctx, req.Email, targetID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("email is already in use by another account")
	}

	admin.Name = req.Name
	admin.Email = req.Email

	if err := s.userRepo.Update(ctx, admin); err != nil {
		return nil, err
	}

	res := dto.ToUserResponse(admin)
	return &res, nil
}

func (s *authService) UpdateStatus(ctx context.Context, targetID int64, callerID int64, req dto.UpdateStatusRequest) (*dto.UserResponse, error) {
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if req.Status != models.StatusActive && req.Status != models.StatusInactive {
		return nil, errors.New("status must be either ACTIVE or INACTIVE")
	}

	if callerID == targetID && req.Status == models.StatusInactive {
		return nil, ErrCannotSelfDeactivate
	}

	admin, err := s.userRepo.FindByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if admin == nil || admin.Role != models.RoleAdmin {
		return nil, ErrUserNotFound
	}

	admin.Status = req.Status
	if err := s.userRepo.Update(ctx, admin); err != nil {
		return nil, err
	}

	res := dto.ToUserResponse(admin)
	return &res, nil
}

func (s *authService) GetProfile(ctx context.Context, userID int64) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	res := dto.ToUserResponse(user)
	return &res, nil
}

func (s *authService) GenerateToken(user *models.User) (string, error) {
	now := time.Now()
	claims := UserClaims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		Name:   user.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "vpspulse-auth",
			Subject:   user.Email,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

func (s *authService) ValidateToken(tokenStr string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &UserClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
