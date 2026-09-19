package handler

// Task 11：user + invite 埋点集成测试（spec §3.2 规则 1-5 / spec §8 清单）。
// fixture 复用 admin_users_test.go（builtin，真实 UserService over sqlite）与
// admin_users_casdoor_test.go（fakeUserDirectory），经 T4 同款 sqlite 包装器
// 追加 audit.Log 断言。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"control-panel/internal/domain/audit"
	authdom "control-panel/internal/domain/auth"

	"github.com/stretchr/testify/require"
)

// patchAdminUser 对 builtin admin env 发起 PATCH /users/:id。
func patchAdminUser(t *testing.T, r http.Handler, id uint64, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/users/%d", id), bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestAuditBuiltinUpdateUserPartialSuccess：spec §3.2 规则 1——同请求内
// role 成功 + status 非法失败 → role 事件已落库（部分成功不漏审），status
// 不落库；随后合法 disable 的 from/to 取 receipt 的 Effective 值。
func TestAuditBuiltinUpdateUserPartialSuccess(t *testing.T) {
	r, users, db := newAdminTestEnv(t)
	_, err := users.CreateInitialAdmin("Passw0rd!")
	require.NoError(t, err)
	member, err := users.Create("member1", "abcd1234", "", authdom.RoleMember)
	require.NoError(t, err)

	w := patchAdminUser(t, r, member.ID, map[string]string{"role": "maintainer", "status": "bogus"})
	require.Equal(t, http.StatusBadRequest, w.Code, "非法 status → 400")

	var roleN, statusN int64
	require.NoError(t, db.Model(&audit.Log{}).Where("action = ?", audit.ActionUpdateRole).Count(&roleN).Error)
	require.NoError(t, db.Model(&audit.Log{}).Where("action = ?", audit.ActionUpdateStatus).Count(&statusN).Error)
	require.EqualValues(t, 1, roleN, "role 已生效：不得因同请求 status 失败而漏审")
	require.EqualValues(t, 0, statusN, "status 未生效：不得记录")

	// 随后合法 disable → builtin status 事件（builtin 下 Effective==Status）
	w = patchAdminUser(t, r, member.ID, map[string]string{"status": "disabled"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rows := rowsOf(t, db, audit.ActionUpdateStatus)
	require.Len(t, rows, 1)
	require.Contains(t, rows[0].Detail, `"from":"active"`)
	require.Contains(t, rows[0].Detail, `"to":"disabled"`)
}

// TestAuditBuiltinUpdateRoleChangeDetailFromReceipt：from/to 取自 receipt
// （member→maintainer），非 handler 预读；幂等同值提交同样如实记录（T7 裁决：
// from==to 的成功 receipt）。
func TestAuditBuiltinUpdateRoleChangeDetailFromReceipt(t *testing.T) {
	r, users, db := newAdminTestEnv(t)
	_, err := users.CreateInitialAdmin("Passw0rd!")
	require.NoError(t, err)
	member, err := users.Create("member1", "abcd1234", "", authdom.RoleMember)
	require.NoError(t, err)

	w := patchAdminUser(t, r, member.ID, map[string]string{"role": "maintainer"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	rows := rowsOf(t, db, audit.ActionUpdateRole)
	require.Len(t, rows, 1)
	require.Equal(t, audit.StatusSuccess, rows[0].Status)
	require.Equal(t, audit.TargetUser, rows[0].TargetType)
	require.Equal(t, strconv.FormatUint(member.ID, 10), rows[0].TargetID)
	require.Equal(t, "member1", rows[0].TargetName) // 展示名 best-effort 预读（仅显示，不用于判定）
	require.Contains(t, rows[0].Detail, `"from":"member"`)
	require.Contains(t, rows[0].Detail, `"to":"maintainer"`)
	require.Equal(t, "1", rows[0].UserID)         // actor：withActor 注入 user_id=1
	require.Equal(t, "default", rows[0].TenantID) // 生产由 JWT 中间件回填

	// 幂等同值更新（maintainer→maintainer）→ 成功 receipt（from==to）→ 同样记录
	w = patchAdminUser(t, r, member.ID, map[string]string{"role": "maintainer"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rows = rowsOf(t, db, audit.ActionUpdateRole)
	require.Len(t, rows, 2)
	require.Contains(t, rows[1].Detail, `"from":"maintainer"`)
	require.Contains(t, rows[1].Detail, `"to":"maintainer"`)
}

// TestAuditCasdoorImplicitApprovalHiddenPending：spec §3.2 规则 2——本地
// pending 用户经 UpdateRole 隐式激活（receipt Status pending→active）→ 补记
// update_status，from/to 用本地权威值。
func TestAuditCasdoorImplicitApprovalHiddenPending(t *testing.T) {
	dir := &fakeUserDirectory{updateRoleRcpt: &authdom.MutationReceipt{
		RoleBefore:    authdom.RoleMember,
		RoleAfter:     authdom.RoleMaintainer,
		StatusBefore:  authdom.StatusPending,
		StatusAfter:   authdom.StatusActive,
		RemoteApplied: true,
		LocalApplied:  true,
	}}
	r, db := setupCasdoorUserRouter(dir, trivialLoginURLFn)

	w := casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"role": "maintainer"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	roleRows := rowsOf(t, db, audit.ActionUpdateRole)
	statusRows := rowsOf(t, db, audit.ActionUpdateStatus)
	require.Len(t, roleRows, 1)
	require.Len(t, statusRows, 1, "隐式审批不得漏审：pending→active 须补记 update_status")
	require.Contains(t, statusRows[0].Detail, `"from":"pending"`) // 本地权威值
	require.Contains(t, statusRows[0].Detail, `"to":"active"`)
	require.Equal(t, audit.StatusSuccess, statusRows[0].Status)
	require.Equal(t, "u1", roleRows[0].TargetID)
	require.Equal(t, "u1", roleRows[0].TargetName) // casdoor 侧 TargetName 用 id
}

// TestAuditCasdoorPartialRemoteApplied：spec §3.2 规则 3——远端已生效、本地
// 失败 → user.update_role 记为 partial（远端变更不得漏审）。
func TestAuditCasdoorPartialRemoteApplied(t *testing.T) {
	dir := &fakeUserDirectory{
		updateRoleErr: errors.New("local projection write failed"),
		updateRoleRcpt: &authdom.MutationReceipt{
			RoleBefore:    authdom.RoleMember,
			RoleAfter:     authdom.RoleMaintainer,
			RemoteApplied: true,
			LocalApplied:  false,
		},
	}
	r, db := setupCasdoorUserRouter(dir, trivialLoginURLFn)

	w := casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"role": "maintainer"})
	require.Equal(t, http.StatusBadGateway, w.Code, "非 sentinel 错误 → 502")

	rows := rowsOf(t, db, audit.ActionUpdateRole)
	require.Len(t, rows, 1, "远端已生效的 partial 变更不得漏审")
	require.Equal(t, audit.StatusPartial, rows[0].Status)
	require.Contains(t, rows[0].Detail, `"from":"member"`)
	require.Contains(t, rows[0].Detail, `"to":"maintainer"`)
}

// TestAuditSetDisabledEffectiveTransition：casdoor status 事件的 from/to 取
// SetDisabled receipt 的 Effective 值（管理视图合成状态）。
func TestAuditSetDisabledEffectiveTransition(t *testing.T) {
	dir := &fakeUserDirectory{setDisabledRcpt: &authdom.MutationReceipt{
		EffectiveStatusBefore: authdom.StatusActive,
		EffectiveStatusAfter:  authdom.StatusDisabled,
	}}
	r, db := setupCasdoorUserRouter(dir, trivialLoginURLFn)

	w := casdoorDo(r, "PATCH", "/admin/users/u1", map[string]string{"status": "disabled"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	rows := rowsOf(t, db, audit.ActionUpdateStatus)
	require.Len(t, rows, 1)
	require.Equal(t, audit.StatusSuccess, rows[0].Status)
	require.Contains(t, rows[0].Detail, `"from":"active"`)
	require.Contains(t, rows[0].Detail, `"to":"disabled"`)
}

// TestAuditResetPasswordAndInvitesAndLoginURL：reset_password（无明文
// Detail）/ invite.create（role+expiresInDays，不落码片段）/ invite.revoke
// （数字 ID 无 Detail）/ login_url（TargetType=system、TargetID=租户）。
func TestAuditResetPasswordAndInvitesAndLoginURL(t *testing.T) {
	// ---- builtin：reset / invite.create / invite.revoke ----
	r, users, db := newAdminTestEnv(t)
	_, err := users.CreateInitialAdmin("Passw0rd!")
	require.NoError(t, err)
	member, err := users.Create("member1", "abcd1234", "", authdom.RoleMember)
	require.NoError(t, err)

	// reset password → user.reset_password，无明文 Detail
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/users/%d/reset-password", member.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rows := rowsOf(t, db, audit.ActionResetPassword)
	require.Len(t, rows, 1)
	require.Equal(t, audit.TargetUser, rows[0].TargetType)
	require.Equal(t, strconv.FormatUint(member.ID, 10), rows[0].TargetID)
	require.Equal(t, "member1", rows[0].TargetName)
	require.Empty(t, rows[0].Detail, "不得记录明文密码")

	// create invite → 数字 TargetID + role/expiresInDays，无码片段
	body, err := json.Marshal(map[string]any{"role": "member", "note": "审计", "expiresInDays": 3})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/invites", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	invRows := rowsOf(t, db, audit.ActionInviteCreate)
	require.Len(t, invRows, 1)
	require.Equal(t, audit.TargetInvite, invRows[0].TargetType)
	require.Equal(t, "1", invRows[0].TargetID, "TargetID 为数字邀请 ID")
	require.Contains(t, invRows[0].Detail, `"role":"member"`)
	require.Contains(t, invRows[0].Detail, `"expiresInDays":3`)
	require.NotContains(t, invRows[0].Detail, "inv_", "不得记录邀请码片段")

	// revoke → 数字 TargetID、无 Detail
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/invites/1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	revRows := rowsOf(t, db, audit.ActionInviteRevoke)
	require.Len(t, revRows, 1)
	require.Equal(t, audit.TargetInvite, revRows[0].TargetType)
	require.Equal(t, "1", revRows[0].TargetID)
	require.Empty(t, revRows[0].Detail)

	// 默认有效期路径（PR #150 审查 P2）：省略 expiresInDays（service 规范化为
	// 7 天），审计必须记录实际生效的 7 而非原始 0
	body, err = json.Marshal(map[string]any{"role": "member"})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/invites", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	invRows = rowsOf(t, db, audit.ActionInviteCreate)
	require.Len(t, invRows, 2)
	require.Contains(t, invRows[1].Detail, `"expiresInDays":7`, "默认有效期审计记录规范化结果（7 天），非原始 0")

	// ---- casdoor：login_url → TargetType=system、TargetID=租户 ----
	cr, cdb := setupCasdoorUserRouter(&fakeUserDirectory{}, trivialLoginURLFn)
	cw := casdoorDo(cr, "GET", "/admin/users/login-url", nil)
	require.Equal(t, http.StatusOK, cw.Code, cw.Body.String())
	lrows := rowsOf(t, cdb, audit.ActionLoginURL)
	require.Len(t, lrows, 1)
	require.Equal(t, audit.TargetSystem, lrows[0].TargetType)
	require.Equal(t, "tenant-a", lrows[0].TargetID)
	require.Empty(t, lrows[0].TargetName)
}
