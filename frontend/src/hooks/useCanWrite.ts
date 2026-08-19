import { useAuthStore } from '@/stores/auth'

/**
 * 当前用户是否拥有管理台写权限（admin | maintainer）。
 *
 * 后端 /api/v1/admin 已拆分为 adminWrite（admin|maintainer）与 adminRead
 * （admin|maintainer|member）：member 只读。前端据此按角色收敛新建/编辑/删除/
 * 部署/探活等写操作按钮（纯 UX 层），后端 403 仍是权限墙。
 * builtin 与 casdoor 两种认证模式共用 user.role 单值（back 端已归一化）。
 */
export function useCanWrite(): boolean {
  const role = useAuthStore((s) => s.user?.role)
  return role === 'admin' || role === 'maintainer'
}
