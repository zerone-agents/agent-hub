// H7.2 UI 插槽 API：聚合查询当前租户的插槽组件列表，以及经平台代理
// 访问扩展自己声明的 GET 授权端点（/api/v1/extensions/{name}/...）。
import apiClient, { unwrapResponse } from './client'

// 与后端 extensionslot.Item 对齐的声明式组件描述。
export interface ExtensionSlotItem {
  slot: string
  component: 'stat-card' | 'link-list' | 'key-value' | 'markdown' | string
  title: string
  order: number
  visible: boolean
  data?: Record<string, unknown>
  dataSource?: { path: string }
  extensionName: string
  extensionId: number
  version: string
}

export interface ExtensionSlotListResult {
  items: ExtensionSlotItem[]
}

export async function fetchExtensionSlots(slot: string): Promise<ExtensionSlotItem[]> {
  const res = await apiClient.get<unknown>('/api/v1/admin/extensions/slots', { params: { slot } })
  return unwrapResponse<ExtensionSlotListResult>(res).items ?? []
}

// 通过平台代理调用扩展声明的 GET 端点；代理已限制 512KB / 5s。
//
// 前端对 path 自行再做一次收口（P2-21）：后端 manifest 校验已要求同源相对
// 路径，但前端不该把"上游已校验"当唯一防线 —— 协议相对写法 `//host/x` 会被
// 浏览器当外站请求发出去（携 cookie 就成了 CSRF/信息外泄通道），
// 绝对 URL 与带协议写法一并拒绝。
export async function fetchExtensionDataSource(path: string): Promise<unknown> {
  if (!isSameOriginApiPath(path)) {
    throw new Error('扩展数据源路径非法：只允许同源 /api/v1/extensions/ 下的相对路径')
  }
  const res = await apiClient.get<unknown>(path)
  return res.data
}

/** 扩展数据源 path 的同源白名单判定（导出供测试直接覆盖边界）。 */
export function isSameOriginApiPath(path: unknown): boolean {
  if (typeof path !== 'string') return false
  const value = path.trim()
  if (value === '') return false
  // 必须是绝对路径（以单个 / 开头，排除协议相对的 //host）
  if (!value.startsWith('/') || value.startsWith('//')) return false
  // 排除反斜杠（部分浏览器等价于 /）与控制字符
  // eslint-disable-next-line no-control-regex -- 这里就是要匹配控制字符本身
  if (value.includes('\\') || /[\u0000-\u001f\u007f]/.test(value)) return false
  return value.startsWith('/api/v1/extensions/')
}
