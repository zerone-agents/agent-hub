package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
	ErrInvalidCredentials     = errors.New("用户名或密码错误")
	ErrLocked                 = errors.New("尝试次数过多，请 15 分钟后再试")
	ErrUsernameTaken          = errors.New("用户名已被占用")
	ErrAlreadyInitialized     = errors.New("系统已初始化")
	ErrLastAdmin              = errors.New("至少保留一个可用管理员")
	ErrWeakPassword           = errors.New("密码至少 8 位，且需包含字母和数字")
	ErrInvalidUsername        = errors.New("用户名需为 3-32 位字母、数字、下划线或连字符")
	ErrSelfOperation          = errors.New("不能对自己执行该操作")
	ErrConcurrentModification = errors.New("用户已被并发修改，请重试")
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
// 返回权威 MutationReceipt（spec §3.2）：builtin 下 Effective==Status、
// RemoteApplied 恒 true；任何错误路径 receipt 为 nil（未生效不记录）。
func (s *UserService) UpdateRole(id, actorID uint64, role string) (*authdom.MutationReceipt, error) {
	if !authdom.IsValidRole(role) {
		return nil, errors.New("非法角色")
	}
	if id == actorID {
		return nil, ErrSelfOperation
	}
	u, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if u.Role == authdom.RoleAdmin && u.Status == authdom.StatusActive && role != authdom.RoleAdmin {
		if err := s.ensureNotLastAdmin(id); err != nil {
			return nil, err
		}
	}
	rows, err := updateColumn(s.db, id, "role", role)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		// MySQL 默认 rowcount=实际改变行数：对 role 已是目标值的用户重复提交，
		// RowsAffected 也是 0——不能直接判未生效。复查 GetByID 消歧（用户裁决
		// 2026-09-11 方案②，spec §3.2）。
		cur, getErr := s.GetByID(id)
		return resolveZeroRows(cur, getErr, "role", role)
	}
	return &authdom.MutationReceipt{
		RoleBefore:            authdom.Role(u.Role),
		RoleAfter:             authdom.Role(role),
		StatusBefore:          authdom.UserStatus(u.Status),
		StatusAfter:           authdom.UserStatus(u.Status),
		EffectiveStatusBefore: authdom.UserStatus(u.Status),
		EffectiveStatusAfter:  authdom.UserStatus(u.Status),
		RemoteApplied:         true,
		LocalApplied:          true,
	}, nil
}

// SetStatus enables/disables a user. Guards: no self-disable, keep last admin.
// 返回权威 MutationReceipt（spec §3.2）：builtin 下 Effective==Status、
// RemoteApplied 恒 true；任何错误路径 receipt 为 nil（未生效不记录）。
func (s *UserService) SetStatus(id, actorID uint64, status string) (*authdom.MutationReceipt, error) {
	if status != authdom.StatusActive && status != authdom.StatusDisabled {
		return nil, errors.New("非法状态")
	}
	if id == actorID && status == authdom.StatusDisabled {
		return nil, ErrSelfOperation
	}
	u, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if u.Role == authdom.RoleAdmin && u.Status == authdom.StatusActive && status == authdom.StatusDisabled {
		if err := s.ensureNotLastAdmin(id); err != nil {
			return nil, err
		}
	}
	rows, err := updateColumn(s.db, id, "status", status)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		// 同 UpdateRole：rows==0 复查 GetByID 消歧（幂等 / 已删除 / 并发写入）。
		cur, getErr := s.GetByID(id)
		return resolveZeroRows(cur, getErr, "status", status)
	}
	return &authdom.MutationReceipt{
		RoleBefore:            authdom.Role(u.Role),
		RoleAfter:             authdom.Role(u.Role),
		StatusBefore:          authdom.UserStatus(u.Status),
		StatusAfter:           authdom.UserStatus(status),
		EffectiveStatusBefore: authdom.UserStatus(u.Status),
		EffectiveStatusAfter:  authdom.UserStatus(status),
		RemoteApplied:         true,
		LocalApplied:          true,
	}, nil
}

// updateColumn：单列 UPDATE，返回 RowsAffected（零行≠错误）。
func updateColumn(db *gorm.DB, id uint64, col, val string) (int64, error) {
	res := db.Model(&authdom.User{}).Where("id = ?", id).Update(col, val)
	return res.RowsAffected, res.Error
}

// resolveZeroRows 消歧 UPDATE RowsAffected==0 的三种结局（用户裁决 2026-09-11
// 方案②，spec §3.2）。生产库是 MySQL 且 DSN 无 clientFoundRows=true，go-sql-driver
// 默认 rowcount=实际改变行数：同值幂等更新 RowsAffected=0 并非未生效，不能直接判
// gorm.ErrRecordNotFound。入参 cur/getErr 为 rows==0 之后复查 GetByID 的结果，
// col 为本次更新的列（"role"/"status"），val 为目标值。三种结局：
//  1. 行仍在且值已等于目标 → 幂等成功：receipt（before==after，按复查到的当前值，
//     RemoteApplied/LocalApplied 均 true）+ nil error——重复提交不得报错；
//  2. 行已不存在（getErr != nil）→ (nil, gorm.ErrRecordNotFound)（真正的并发删除窗口）；
//  3. 行仍在但值不等于目标（并发写入者胜出，我方写入实为 no-op）→
//     (nil, ErrConcurrentModification)。
func resolveZeroRows(cur *authdom.User, getErr error, col, val string) (*authdom.MutationReceipt, error) {
	if getErr != nil {
		return nil, gorm.ErrRecordNotFound
	}
	equal := false
	switch col {
	case "role":
		equal = cur.Role == val
	case "status":
		equal = cur.Status == val
	default:
		return nil, fmt.Errorf("resolveZeroRows: 未知列 %q", col)
	}
	if !equal {
		return nil, ErrConcurrentModification
	}
	return &authdom.MutationReceipt{
		RoleBefore:            authdom.Role(cur.Role),
		RoleAfter:             authdom.Role(cur.Role),
		StatusBefore:          authdom.UserStatus(cur.Status),
		StatusAfter:           authdom.UserStatus(cur.Status),
		EffectiveStatusBefore: authdom.UserStatus(cur.Status),
		EffectiveStatusAfter:  authdom.UserStatus(cur.Status),
		RemoteApplied:         true,
		LocalApplied:          true,
	}, nil
}

// ResetPassword sets a random password and returns the plaintext once.
// 不能对自己重置——自己的密码走自助改密（ChangePassword）。
func (s *UserService) ResetPassword(id, actorID uint64) (string, error) {
	if id == actorID {
		return "", ErrSelfOperation
	}
	if _, err := s.GetByID(id); err != nil {
		return "", err
	}
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	plain := hex.EncodeToString(b)[:16]
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	if err := s.db.Model(&authdom.User{}).Where("id = ?", id).Update("password_hash", string(hash)).Error; err != nil {
		return "", err
	}
	return plain, nil
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
