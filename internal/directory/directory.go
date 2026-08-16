package directory

import (
	"errors"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

var ErrSelfOperation = errors.New("cannot perform this operation on yourself")
var ErrUserNotFound = errors.New("user not found")
var ErrInvalidRole = errors.New("invalid role") // not admin/maintainer/member, or not in mapping

// ManagedUser is the admin-UI projection of a user, backend-agnostic.
type ManagedUser struct {
	ID          string `json:"id"` // casdoor user Id
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Role        string `json:"role"`      // highest normalized: admin|maintainer|member
	Status      string `json:"status"`    // active|disabled
	CreatedAt   string `json:"createdAt"` // casdoor CreatedTime, passed through
}

// UserClient is the narrow slice of casdoorsdk.Client used by
// CasdoorDirectory. Defined here so tests can fake it.
type UserClient interface {
	GetUsers() ([]*casdoorsdk.User, error)
	GetUserByUserId(userId string) (*casdoorsdk.User, error)
	UpdateUserForColumns(user *casdoorsdk.User, columns []string) (bool, error)
}

// CasdoorDirectory manages users through the Casdoor admin API.
type CasdoorDirectory struct {
	client      UserClient
	roleMapping map[string]string // agent-hub role -> casdoor role name
	defaultRole string
}
