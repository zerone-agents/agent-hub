import { Input } from 'antd'
import { MagnifyingGlass } from '@phosphor-icons/react'

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
export default function NameSearch({
  placeholder = '搜索名称',
  onSearch,
  maxWidth = 320,
  realtime = false
}: NameSearchProps) {
  if (realtime) {
    return (
      <Input
        placeholder={placeholder}
        allowClear
        prefix={<MagnifyingGlass size={14} color="var(--text-muted, #999)" />}
        style={{ maxWidth }}
        onChange={(e) => { onSearch(e.target.value.trim()); }}
      />
    )
  }

  return (
    <Input.Search
      placeholder={placeholder}
      allowClear
      style={{ maxWidth }}
      onSearch={(value) => { onSearch(value.trim()); }}
    />
  )
}
