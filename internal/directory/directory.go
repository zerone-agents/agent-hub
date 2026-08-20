package directory

import (
	"errors"

	"control-panel/internal/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

var ErrSelfOperation = errors.New("cannot perform this operation on yourself")
var ErrUserNotFound = errors.New("user not found")
var ErrInvalidRole = errors.New("invalid role") // not admin/maintainer/member

// ErrUpdateRejected is returned when casdoor answers an update request with
// ok=false and no error (e.g. permission denied server-side).
var ErrUpdateRejected = errors.New("casdoor rejected the update")

// ManagedUser is the admin-UI projection of a user, backend-agnostic.
type ManagedUser struct {
	ID          string `json:"id"` // casdoor user Id (= user_identities.external_id)
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Role        string `json:"role"`      // 本地管理的角色：admin|maintainer|member；空 = 未分配（待审批）
	Status      string `json:"status"`    // active|pending|disabled（disabled 由 casdoor is_forbidden 实时合成）
	CreatedAt   string `json:"createdAt"` // 本地成员记录创建时间（RFC3339）
}

// UserClient is the narrow slice of the casdoor admin API used by
// CasdoorDirectory. Since Task 6 roles live in the local membership table;
// only the fields still owned by casdoor (is_admin / is_forbidden / password)
// go through this client.
type UserClient interface {
	GetUsers() ([]*casdoorsdk.User, error)
	GetUserByUserId(userId string) (*casdoorsdk.User, error)
	UpdateUserForColumns(user *casdoorsdk.User, columns []string) (bool, error)
}

// ClientResolver resolves the tenant-scoped casdoor client for a tenant
// (casdoor organization). Multi-tenant: SDK API calls filter by the client's
// OrganizationName, so each tenant needs its own client — see
// auth.ClientForOrg.
type ClientResolver func(tenantID string) UserClient

// CasdoorDirectory serves user management from the local membership table
// (auth.MembershipStore)：角色/审批状态以本地为真实源，仅 is_admin（admin
// 任免双写）、is_forbidden（禁用直通）、password（重置直通）走 casdoor。
type CasdoorDirectory struct {
	resolveClient ClientResolver
	store         auth.MembershipStore
}
