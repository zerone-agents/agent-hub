package services

import (
	"errors"
	"regexp"
	"sync"
	"time"
	"unicode"

	authdom "control-panel/internal/domain/auth"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Sentinel errors for the builtin user system. Login failures are deliberately
// uniform (ErrInvalidCredentials) to avoid user enumeration.
var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrLocked             = errors.New("尝试次数过多，请 15 分钟后再试")
	ErrUsernameTaken      = errors.New("用户名已被占用")
	ErrAlreadyInitialized = errors.New("系统已初始化")
	ErrLastAdmin          = errors.New("至少保留一个可用管理员")
	ErrWeakPassword       = errors.New("密码至少 8 位，且需包含字母和数字")
	ErrInvalidUsername    = errors.New("用户名需为 3-32 位字母、数字、下划线或连字符")
	ErrSelfOperation      = errors.New("不能对自己执行该操作")
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

const (
	bcryptCost    = 12
	maxFailures   = 5
	lockoutWindow = 15 * time.Minute
)

// UserService manages builtin local users and credentials, including per-user
// lockout and last-active-admin protection.
type UserService struct {
	db *gorm.DB

	failuresMu sync.Mutex
	failures   map[string]*failureRecord
}

type failureRecord struct {
	count       int
	lockedUntil time.Time
}

// NewUserService constructs a UserService backed by db.
func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db, failures: map[string]*failureRecord{}}
}

// Initialized reports whether any user exists (i.e. setup has run).
func (s *UserService) Initialized() (bool, error) {
	var count int64
	err := s.db.Model(&authdom.User{}).Count(&count).Error
	return count > 0, err
}

// CreateInitialAdmin creates the fixed-username "admin" account exactly once.
// Concurrent calls are safe: the DB unique constraint on username serializes
// them and a conflict is mapped to ErrAlreadyInitialized.
func (s *UserService) CreateInitialAdmin(password string) (*authdom.User, error) {
	ok, err := s.Initialized()
	if err != nil {
		return nil, err
	}
	if ok {
		return nil, ErrAlreadyInitialized
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, err
	}
	u := &authdom.User{
		Username:     "admin",
		PasswordHash: string(hash),
		DisplayName:  "管理员",
		Role:         authdom.RoleAdmin,
		Status:       authdom.StatusActive,
	}
	if err := s.db.Create(u).Error; err != nil {
		// Concurrent setup: unique-constraint conflict → already initialized.
		return nil, ErrAlreadyInitialized
	}
	return u, nil
}

// Authenticate verifies username+password with per-username lockout. All
// failure modes (unknown user, wrong password, disabled) return
// ErrInvalidCredentials; lockout returns ErrLocked.
func (s *UserService) Authenticate(username, password string) (*authdom.User, error) {
	if s.isLocked(username) {
		return nil, ErrLocked
	}
	var u authdom.User
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		s.recordFailure(username)
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		s.recordFailure(username)
		return nil, ErrInvalidCredentials
	}
	if u.Status != authdom.StatusActive {
		return nil, ErrInvalidCredentials
	}
	s.clearFailures(username)
	return &u, nil
}

// Create registers a user with validation. role must be a builtin role.
func (s *UserService) Create(username, password, displayName, role string) (*authdom.User, error) {
	if !usernameRe.MatchString(username) {
		return nil, ErrInvalidUsername
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	if !authdom.IsValidRole(role) {
		return nil, errors.New("非法角色")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, err
	}
	if displayName == "" {
		displayName = username
	}
	u := &authdom.User{
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		Role:         role,
		Status:       authdom.StatusActive,
	}
	if err := s.db.Create(u).Error; err != nil {
		return nil, ErrUsernameTaken
	}
	return u, nil
}

// ChangePassword verifies the old password then sets the new one.
func (s *UserService) ChangePassword(userID uint64, oldPassword, newPassword string) error {
	var u authdom.User
	if err := s.db.First(&u, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return err
	}
	return s.db.Model(&u).Update("password_hash", string(hash)).Error
}

// List returns all users ordered by id ascending.
func (s *UserService) List() ([]*authdom.User, error) {
	var users []*authdom.User
	err := s.db.Order("id ASC").Find(&users).Error
	return users, err
}

// GetByID loads a user or returns an error.
func (s *UserService) GetByID(id uint64) (*authdom.User, error) {
	var u authdom.User
	if err := s.db.First(&u, id).Error; err != nil {
		return nil, errors.New("用户不存在")
	}
	return &u, nil
}

// Delete removes a user by id. Intended for rollback of a just-created user
// when a follow-up step (e.g. invite consume) fails — not exposed as a general
// admin endpoint (admins disable instead, preserving audit history).
func (s *UserService) Delete(id uint64) error {
	return s.db.Delete(&authdom.User{}, id).Error
}

// UpdateRole changes a user's role. Guards: no self-change, keep last admin.
func (s *UserService) UpdateRole(id, actorID uint64, role string) error {
	if !authdom.IsValidRole(role) {
		return errors.New("非法角色")
	}
	if id == actorID {
		return ErrSelfOperation
	}
	u, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if u.Role == authdom.RoleAdmin && u.Status == authdom.StatusActive && role != authdom.RoleAdmin {
		if err := s.ensureNotLastAdmin(id); err != nil {
			return err
		}
	}
	return s.db.Model(&authdom.User{}).Where("id = ?", id).Update("role", role).Error
}

// SetStatus enables/disables a user. Guards: no self-disable, keep last admin.
func (s *UserService) SetStatus(id, actorID uint64, status string) error {
	if status != authdom.StatusActive && status != authdom.StatusDisabled {
		return errors.New("非法状态")
	}
	if id == actorID && status == authdom.StatusDisabled {
		return ErrSelfOperation
	}
	u, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if u.Role == authdom.RoleAdmin && u.Status == authdom.StatusActive && status == authdom.StatusDisabled {
		if err := s.ensureNotLastAdmin(id); err != nil {
			return err
		}
	}
	return s.db.Model(&authdom.User{}).Where("id = ?", id).Update("status", status).Error
}

// ensureNotLastAdmin errors when no other active admin exists besides excludeID.
func (s *UserService) ensureNotLastAdmin(excludeID uint64) error {
	var count int64
	err := s.db.Model(&authdom.User{}).
		Where("role = ? AND status = ? AND id != ?", authdom.RoleAdmin, authdom.StatusActive, excludeID).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrLastAdmin
	}
	return nil
}

func (s *UserService) isLocked(username string) bool {
	s.failuresMu.Lock()
	defer s.failuresMu.Unlock()
	rec, ok := s.failures[username]
	if !ok {
		return false
	}
	if time.Now().Before(rec.lockedUntil) {
		return true
	}
	if !rec.lockedUntil.IsZero() {
		// Lockout window elapsed — reset the record.
		delete(s.failures, username)
	}
	return false
}

func (s *UserService) recordFailure(username string) {
	s.failuresMu.Lock()
	defer s.failuresMu.Unlock()
	rec, ok := s.failures[username]
	if !ok {
		rec = &failureRecord{}
		s.failures[username] = rec
	}
	rec.count++
	if rec.count >= maxFailures {
		rec.lockedUntil = time.Now().Add(lockoutWindow)
		rec.count = 0
	}
}

func (s *UserService) clearFailures(username string) {
	s.failuresMu.Lock()
	defer s.failuresMu.Unlock()
	delete(s.failures, username)
}

func validatePassword(pw string) error {
	if len(pw) < 8 {
		return ErrWeakPassword
	}
	var hasLetter, hasDigit bool
	for _, r := range pw {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}
