import { Input } from 'antd'
import { MagnifyingGlassIcon } from '@phosphor-icons/react'
import { useTranslation } from 'react-i18next'

export interface NameSearchProps {
  placeholder?: string
  onSearch: (value: string) => void
  /** maxWidth in px, default 320 */
  maxWidth?: number
  /** 实时搜索模式：输入即触发，无需回车。默认 false（回车/点按钮触发） */
  realtime?: boolean
}

/**
 * 名称搜索框，统一 allowClear + trim + maxWidth 样式。
 * onSearch 回调收到的值已 trim。
 * realtime=true 时输入即生效（适合前端过滤），false 时回车/点按钮生效（适合服务端搜索）。
 */
// placeholder 默认中文值走哨兵模式（参数默认值作用域拿不到 hook 的 t）。
export default function NameSearch({
  placeholder,
  onSearch,
  maxWidth = 320,
  realtime = false
}: NameSearchProps) {
  const { t } = useTranslation()
  const ph = placeholder ?? t('components.nameSearch.placeholder')
  if (realtime) {
    return (
      <Input
        placeholder={ph}
        allowClear
        prefix={<MagnifyingGlassIcon size={14} color="var(--text-muted, #999)" />}
        style={{ maxWidth }}
        onChange={(e) => { onSearch(e.target.value.trim()); }}
      />
    )
  }

  return (
    <Input.Search
      placeholder={ph}
      allowClear
      style={{ maxWidth }}
      onSearch={(value) => { onSearch(value.trim()); }}
    />
  )
}
