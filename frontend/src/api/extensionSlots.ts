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
export async function fetchExtensionDataSource(path: string): Promise<unknown> {
  const res = await apiClient.get<unknown>(path)
  return res.data
}
