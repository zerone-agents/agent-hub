import type { Tool } from '@/api/tools'
// 纯函数上下文：分组 label 直调 i18next
import i18next from '@/i18n'

export interface ToolOption {
  value: string
  label: string
  disabled?: boolean
}

export interface ToolOptionGroup {
  label: string
  options: ToolOption[]
}

// 统一选择器按来源分组（issue #88）：内置/自定义两组；missing 存量工具原位
// 展示并说明原因——未选中的禁选（不可新增挂载），已关联的保持可操作（用户
// 可明确取消选择；后端仍是最终约束）。
export function buildToolOptions(
  tools: Tool[],
  selectedTools: string[],
  defaultToolNames: Set<string>
): ToolOptionGroup[] {
  const builtin: ToolOption[] = []
  const custom: ToolOption[] = []
  for (const tl of tools) {
    const isMissingCustom = tl.source === 'custom' && tl.artifactStatus === 'missing'
    const option: ToolOption = {
      value: tl.name,
      label: isMissingCustom ? i18next.t('agents.toolOptions.missingFile', { name: tl.name }) : tl.name,
      disabled:
        defaultToolNames.has(tl.name) ||
        (isMissingCustom && !selectedTools.includes(tl.name))
    }
    if (tl.source === 'builtin') {
      builtin.push(option)
    } else {
      custom.push(option)
    }
  }
  const groups: ToolOptionGroup[] = []
  if (builtin.length > 0) groups.push({ label: i18next.t('tools.builtinSection'), options: builtin })
  if (custom.length > 0) groups.push({ label: i18next.t('tools.customSection'), options: custom })
  return groups
}
