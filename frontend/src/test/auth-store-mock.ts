import { vi } from 'vitest'

/**
 * 可变的 authUser（默认 admin）：既有断言依赖写操作按钮可见，
 * member 分支用例在测试内切换 role（beforeEach 重置回 admin）。
 *
 * 通过 vi.hoisted 定义并从工厂中引用：vi.mock 工厂会被提升（hoist）到
 * import 之前执行，普通模块级/import 的变量在工厂执行时尚未初始化。
 * 注意不能 export hoisted 变量本身（vitest 会报 "Cannot export hoisted
 * variable"），故通过 setAuthRole/getAuthUser 访问。
 */
const authUser = vi.hoisted(() => ({
  user: { id: '1', name: 'admin', email: 'admin@zerone.run', role: 'admin' as string }
}))

/** 切换当前 mock 用户角色（测试内 beforeEach 重置 / member 用例切换）。 */
export function setAuthRole(role: string) {
  authUser.user = { ...authUser.user, role }
}

/** 读取当前 mock 用户（断言兜底用）。 */
export function getAuthUser() {
  return authUser.user
}

/** @/stores/auth 的 mock 模块内容：selector 形式，与 PendingApprovalPage.test.tsx 既有风格一致。 */
export function createAuthStoreMock() {
  return {
    useAuthStore: (selector: (s: {
      user: { id: string; name: string; email: string; role: string } | null
      setUser: () => void
      loginWithPassword: () => Promise<void>
      login: () => void
      logout: () => Promise<void>
    }) => unknown) => selector({
      user: authUser.user,
      setUser: vi.fn(),
      loginWithPassword: vi.fn(),
      login: vi.fn(),
      logout: vi.fn()
    })
  }
}