import type { Icon } from '@phosphor-icons/react'
import {
  Gauge,
  Robot,
  Wrench,
  PlugsConnected,
  Sparkle,
  Cube,
  FilmSlate,
  Books,
  Chats
} from '@phosphor-icons/react'

export interface NavItem {
  id: string
  label: string
  path: string
  icon: Icon
}

/**
 * Static navigation items. Not reactive — derived from constants, not a store.
 */
export const NAV_ITEMS: readonly NavItem[] = [
  { id: 'dashboard', label: '仪表盘', path: '/dashboard', icon: Gauge },
  { id: 'agents', label: 'Agent管理', path: '/agents', icon: Robot },
  { id: 'tools', label: '工具管理', path: '/tools', icon: Wrench },
  { id: 'mcps', label: 'MCP配置', path: '/mcps', icon: PlugsConnected },
  { id: 'skills', label: '技能管理', path: '/skills', icon: Sparkle },
  { id: 'providers', label: '模型管理', path: '/providers', icon: Cube },
  { id: 'knowledge', label: '知识库管理', path: '/knowledge', icon: Books },
  { id: 'scenes', label: '场景管理', path: '/scenes', icon: FilmSlate },
  { id: 'chat', label: '聊天记录', path: '/chat', icon: Chats }
] as const

const SETTINGS_LABELS: Record<string, string> = {
  'cli-tokens': 'CLI Tokens',
  aigc: 'AIGC 标识配置'
}

export interface BreadcrumbItem {
  label: string
  /** Route to navigate to when clicked. Absent = current page / non-navigable. */
  path?: string
}

/**
 * Breadcrumb items for a pathname, e.g. '/settings/aigc' ->
 * [{label:'首页',path:'/dashboard'}, {label:'设置'}, {label:'AIGC 标识配置'}].
 * The last item is the current page and never has a path.
 * `knowledgeName` replaces the generic '详情' segment on knowledge detail routes.
 */
export function getBreadcrumbs(pathname: string, knowledgeName?: string): BreadcrumbItem[] {
  const home: BreadcrumbItem = { label: '首页', path: '/dashboard' }
  const segments = pathname.split('/').filter(Boolean)
  const [first, second] = segments
  if (!first || first === 'dashboard') return [{ label: '首页' }]

  if (first === 'settings') {
    const page = second ? SETTINGS_LABELS[second] : undefined
    return page ? [home, { label: '设置' }, { label: page }] : [home, { label: '设置' }]
  }

  const item = NAV_ITEMS.find((i) => i.path === `/${first}`)
  if (!item) return [{ label: '首页' }]
  if (!second) return [home, { label: item.label }]
  return [home, { label: item.label, path: item.path }, { label: knowledgeName ?? '详情' }]
}
