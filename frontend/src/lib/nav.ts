import type { Icon } from '@phosphor-icons/react'
// 纯函数上下文：getBreadcrumbs 直调 i18next 输出用户面文案
import i18next from '@/i18n'
import {
  GaugeIcon,
  RobotIcon,
  WrenchIcon,
  PlugsConnectedIcon,
  SparkleIcon,
  CubeIcon,
  FilmSlateIcon,
  BooksIcon,
  ChatsIcon
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
  { id: 'dashboard', label: 'nav.dashboard', path: '/dashboard', icon: GaugeIcon },
  { id: 'agents', label: 'nav.agents', path: '/agents', icon: RobotIcon },
  { id: 'tools', label: 'nav.tools', path: '/tools', icon: WrenchIcon },
  { id: 'mcps', label: 'nav.mcps', path: '/mcps', icon: PlugsConnectedIcon },
  { id: 'skills', label: 'nav.skills', path: '/skills', icon: SparkleIcon },
  { id: 'providers', label: 'nav.providers', path: '/providers', icon: CubeIcon },
  { id: 'knowledge', label: 'nav.knowledge', path: '/knowledge', icon: BooksIcon },
  { id: 'scenes', label: 'nav.scenes', path: '/scenes', icon: FilmSlateIcon },
  { id: 'chat', label: 'nav.chat', path: '/chat', icon: ChatsIcon }
] as const

// settings 子页面包屑标签。值统一过 i18next.t()（cli-tokens 为英文品牌词，
// 无对应资源 key，t() 按 missing-key 透传原样输出）。
const SETTINGS_LABELS: Record<string, string> = {
  'cli-tokens': 'CLI Tokens',
  aigc: 'nav.settingsLabels.aigc',
  users: 'nav.settingsLabels.users',
  'audit-logs': 'nav.settingsLabels.auditLogs'
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
  // 纯函数直调 i18next（非组件上下文）：输出即用户面文案——
  // 消费方（AppHeader breadcrumb）无需再包 t()，nav.test 断言中文零改动。
  const home: BreadcrumbItem = { label: i18next.t('nav.home'), path: '/dashboard' }
  const segments = pathname.split('/').filter(Boolean)
  const [first, second] = segments
  if (!first || first === 'dashboard') return [{ label: i18next.t('nav.home') }]

  if (first === 'settings') {
    const page = second ? SETTINGS_LABELS[second] : undefined
    return page
      ? [home, { label: i18next.t('nav.settings') }, { label: i18next.t(page) }]
      : [home, { label: i18next.t('nav.settings') }]
  }

  const item = NAV_ITEMS.find((i) => i.path === `/${first}`)
  if (!item) return [{ label: i18next.t('nav.home') }]
  if (!second) return [home, { label: i18next.t(item.label) }]
  return [home, { label: i18next.t(item.label), path: item.path }, { label: knowledgeName ?? i18next.t('nav.detail') }]
}
