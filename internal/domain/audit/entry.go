package audit

import (
	"strings"

	"control-panel/internal/domain/aigc"
)

// Actor 是事件的行动者。嵌入 Entry 时，非空字段覆盖 Recorder 从
// gin context 提取的缺省值（auth.login/setup 等未过 JWT 的点位，spec §5.2）。
type Actor struct {
	TenantID  string
	UserID    string
	UserName  string
	RemoteIP  string
	UserAgent string
}

type Entry struct {
	Actor      Actor
	Category   Category
	Action     Action
	TargetType TargetType
	TargetID   string
	TargetName string
	Status     Status
	Detail     any // nil 或强类型 Detail struct
}

// categoryOf 从 Action 派生 Category（spec §3 分类表）。
// 规则 = action 前缀，唯一例外 cli_token.* → token（前缀与 category 名不同）。
// brief 的 SimpleEvent 实现漏设 Category，而 TestSimpleEventDefaults 要求派生，
// 依 TDD 以测试为准补此映射。
func categoryOf(action Action) Category {
	switch action {
	case ActionLogin, ActionLogout, ActionSetup, ActionPasswordChange:
		return CatAuth
	case ActionUpdateRole, ActionUpdateStatus, ActionResetPassword, ActionLoginURL:
		return CatUser
	case ActionInviteCreate, ActionInviteRevoke:
		return CatInvite
	case ActionRevealKey, ActionRuntimeConfig, ActionSyncMultirag:
		return CatProvider
	case ActionDeploy, ActionStop, ActionStart, ActionUndeploy, ActionDelete:
		return CatAgent
	case ActionCliTokenIssue, ActionCliTokenRevoke:
		return CatToken
	case ActionAigcSave, ActionAigcDelete, ActionAigcRotateKey:
		return CatAigc
	default:
		return Category(strings.SplitN(string(action), ".", 2)[0])
	}
}

func SimpleEvent(a Actor, action Action, tt TargetType, targetID, targetName string) Entry {
	return Entry{Actor: a, Category: categoryOf(action), Action: action, TargetType: tt, TargetID: targetID, TargetName: targetName, Status: StatusSuccess}
}

func LoginEvent(a Actor, username, org string, st Status, reason string) Entry {
	return Entry{
		Actor: a, Category: CatAuth, Action: ActionLogin, TargetType: TargetSystem,
		Status: st, Detail: LoginDetail{Username: username, Org: org, Reason: reason},
	}
}

func RoleChangedEvent(a Actor, targetID, targetName, from, to string, st Status) Entry {
	return Entry{
		Actor: a, Category: CatUser, Action: ActionUpdateRole, TargetType: TargetUser,
		TargetID: targetID, TargetName: targetName, Status: st,
		Detail: ChangeDetail{Field: "role", From: from, To: to},
	}
}

func StatusChangedEvent(a Actor, targetID, targetName, from, to string) Entry {
	return Entry{
		Actor: a, Category: CatUser, Action: ActionUpdateStatus, TargetType: TargetUser,
		TargetID: targetID, TargetName: targetName, Status: StatusSuccess,
		Detail: ChangeDetail{Field: "status", From: from, To: to},
	}
}

func InviteCreatedEvent(a Actor, inviteID, role string, expiresInDays int) Entry {
	return Entry{
		Actor: a, Category: CatInvite, Action: ActionInviteCreate, TargetType: TargetInvite,
		TargetID: inviteID, Status: StatusSuccess,
		Detail: InviteDetail{Role: role, ExpiresInDays: expiresInDays},
	}
}

func RuntimeConfigEvent(a Actor, count int) Entry {
	return Entry{
		Actor: a, Category: CatProvider, Action: ActionRuntimeConfig, TargetType: TargetSystem,
		Status: StatusSuccess, Detail: CountDetail{Count: count},
	}
}

func AigcSavedEvent(a Actor, changed []aigc.AigcConfigField) Entry {
	return Entry{
		Actor: a, Category: CatAigc, Action: ActionAigcSave, TargetType: TargetAigcConfig,
		Status: StatusSuccess, Detail: AigcConfigDetail{ChangedFields: changed},
	}
}
