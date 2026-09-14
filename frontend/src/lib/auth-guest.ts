import type { User } from '@/api/auth'

/**
 * 前端有效 guest 谓词（镜像后端 jwtutil.IsGuest 的 web 可见部分，spec 5.2）：
 * 显式 guest 角色（任意 mode）或 casdoor 空 roles（隐式 guest）。
 */
export function isGuestUser(
  user: Pick<User, 'role'> | null | undefined,
  mode?: string | null
): boolean {
  if (!user) return false
  if (user.role === 'guest') return true
  return mode === 'casdoor' && !user.role
}
