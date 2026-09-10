import React, { useEffect } from 'react'
import { Modal, Form, Input, Select, InputNumber, Switch, AutoComplete, Button } from 'antd'
import { XIcon, CaretDownIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Agent, AgentConfig, BehaviorProfile } from '@/api/agents'
import { useCreateAgent, useUpdateAgent, useAgents } from '@/queries/useAgents'
import { usePersonalities } from '@/queries/usePersonalities'
import { agentIdentifierFormRules } from '@/utils/identifier'
import { AGENT_ICON_OPTIONS, PRESET_COLORS, PRESET_BG_COLORS } from '@/utils/agent-icons'
import { getIconComponent, lightenHex } from '@/utils/icons'
import BehaviorProfileEditor from './BehaviorProfileEditor'
import { cloneBehaviorProfile, DEFAULT_BEHAVIOR_PROFILE } from './behaviorProfile'

interface ToggleItemProps {
  title: string
  desc: string
  checked?: boolean
  onChange?: (checked: boolean) => void
}

function ToggleItem({ title, desc, checked, onChange }: ToggleItemProps) {
  const { styles } = useStyles()
  return (
    <div className={styles.toggleRow} onClick={() => onChange?.(!checked)}>
      <Switch size="small" checked={checked}
        onChange={(v) => onChange?.(v)}
        onClick={(_checked, ev) => { ev.stopPropagation(); }} />
      <div className={styles.toggleText}>
        <span className={styles.toggleName}>{title}</span>
        <span className={styles.toggleDesc}>{desc}</span>
      </div>
    </div>
  )
}

const useStyles = createStyles(({ css }) => ({
  head: css`
    display: flex; justify-content: space-between; align-items: center;
    padding: 18px 24px; border-bottom: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  title: css`font-size: 18px; font-weight: 600; color: var(--text); letter-spacing: -0.02em;`,
  closeBtn: css`
    width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
    border: none; background: var(--ink-subtle); border-radius: 4px;
    color: var(--text-tertiary); cursor: pointer; transition: all 0.15s;
    &:hover { background: var(--ink-light); color: var(--text); }
  `,
  body: css`padding: 20px 24px 8px; max-height: 60vh; overflow-y: auto;`,
  section: css`
    font-size: 11px; font-weight: 600; color: var(--text-muted);
    text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 14px;
  `,
  row: css`display: grid; grid-template-columns: 1fr 1fr; gap: 16px;`,
  iconGrid: css`
    display: grid; grid-template-columns: repeat(10, 1fr); gap: 4px;
  `,
  iconCell: css`
    width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
    border-radius: 4px; cursor: pointer; transition: all 0.15s;
    border: 1px solid transparent; background: transparent;
    &:hover { background: var(--ink-subtle); }
  `,
  iconCellActive: css`border-width: 1px; border-style: solid;`,
  colorStrip: css`display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 8px;`,
  colorDot: css`
    width: 20px; height: 20px; border-radius: 50%; cursor: pointer;
    border: 2px solid transparent; transition: border-color 0.15s;
    &:hover { border-color: var(--ink-light); }
  `,
  colorDotOn: css`border-color: var(--ink) !important;`,
  toggleRow: css`
    display: flex; align-items: center; gap: 12px; padding: 12px 0;
    cursor: pointer;
    &:hover { background: var(--ink-subtle); border-radius: 4px; padding: 12px 8px; margin: 0 -8px; }
  `,
  toggleText: css`display: flex; flex-direction: column;`,
  toggleName: css`font-size: 13px; font-weight: 500; color: var(--text);`,
  toggleDesc: css`font-size: 11px; color: var(--text-muted);`,
  foot: css`
    display: flex; justify-content: flex-end; gap: 10px;
    padding: 14px 24px; border-top: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  personalityShell: css`
    overflow: hidden; border: 1px solid color-mix(in srgb, var(--foreground) 9%, transparent);
    border-radius: 9px; background: color-mix(in srgb, var(--background) 97%, var(--primary) 3%);
  `,
  personalityHead: css`
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px;
    align-items: center; padding: 14px 16px; border-bottom: 1px solid var(--border);
  `,
  personalityLabel: css`display: block; margin-bottom: 4px; color: var(--text); font-size: 13px; font-weight: 650;`,
  personalityHint: css`display: block; color: var(--text-muted); font-size: 11px; line-height: 1.5;`,
  personalityVersion: css`color: var(--primary); font: 650 10px/1 ui-monospace, SFMono-Regular, Menlo, monospace;`,
  personalityBody: css`padding: 14px 16px 4px;`,
  personalityPrompt: css`
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace !important;
    line-height: 1.65 !important;
  `,
  personalityFoot: css`
    margin: -6px 0 14px; padding-left: 10px;
    border-left: 2px solid color-mix(in srgb, var(--primary) 38%, transparent);
    color: var(--text-muted); font-size: 10px; line-height: 1.5;
  `
}))

interface AgentFormProps {
  open: boolean
  editingAgent: Agent | null
  onClose: () => void
}

interface FormValues {
  name: string
  permissionMode: string
  titleZh: string
  titleEn: string
  descriptionZh: string
  descriptionEn: string
  iconName: string
  iconColor: string
  iconBgColor: string
  maxTurns: number
  maxSessionQueries?: number
  disallowedTools?: string[]
  systemPrompt: string
  desktopEnabled: boolean
  mobileEnabled: boolean
  isDefault: boolean
  group: string
  behaviorProfile: BehaviorProfile
  personalityTemplateName: string
  personalityTemplateVersion: number
  personalityPrompt: string
}

export default function AgentForm({ open, editingAgent, onClose }: AgentFormProps) {
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const createAgent = useCreateAgent()
  const updateAgent = useUpdateAgent()
  const submitting = createAgent.isPending || updateAgent.isPending
  const { data: allAgents = [] } = useAgents()
  const { data: personalities = [] } = usePersonalities()

  // 提取去重后的 group 值
  const groupOptions = React.useMemo(() => {
    const groups = new Set<string>()
    allAgents.forEach(agent => {
      if (agent.group) {
        groups.add(agent.group)
      }
    })
    return Array.from(groups)
      .sort((a, b) => a.localeCompare(b))
      .map(g => ({ value: g, label: g }))
  }, [allAgents])

  const iconName = Form.useWatch('iconName', form)
  const iconColor = Form.useWatch('iconColor', form)
  const iconBgColor = Form.useWatch('iconBgColor', form)
  const personalityTemplateName = Form.useWatch('personalityTemplateName', form)
  const personalityTemplateVersion = Form.useWatch('personalityTemplateVersion', form)
  const personalityPrompt = Form.useWatch('personalityPrompt', form)
  const selectedPersonality = personalities.find((item) => item.name === personalityTemplateName)
  const personalityModified = Boolean(selectedPersonality && personalityPrompt.trim() !== selectedPersonality.prompt.trim())

  useEffect(() => {
    if (open) {
      if (editingAgent) {
        form.setFieldsValue({
          name: editingAgent.name,
          permissionMode: editingAgent.config.permissionMode ?? 'auto',
          titleZh: editingAgent.config.title?.zh ?? '',
          titleEn: editingAgent.config.title?.en ?? '',
          descriptionZh: editingAgent.config.description?.zh ?? '',
          descriptionEn: editingAgent.config.description?.en ?? '',
          iconName: editingAgent.config.iconName ?? '',
          iconColor: editingAgent.config.iconColor ?? '',
          iconBgColor: editingAgent.config.iconBgColor ?? '',
          maxTurns: editingAgent.config.maxTurns ?? 50,
          maxSessionQueries: editingAgent.config.maxSessionQueries,
          disallowedTools: editingAgent.config.disallowedTools ?? [],
          systemPrompt: editingAgent.config.systemPrompt ?? '',
          desktopEnabled: editingAgent.desktopEnabled ?? false,
          mobileEnabled: editingAgent.mobileEnabled ?? false,
          isDefault: editingAgent.isDefault ?? false,
          group: editingAgent.group ?? '',
          behaviorProfile: cloneBehaviorProfile(editingAgent.config.behaviorProfile ?? DEFAULT_BEHAVIOR_PROFILE),
          personalityTemplateName: editingAgent.config.personalityTemplateName ?? '',
          personalityTemplateVersion: editingAgent.config.personalityTemplateVersion ?? 0,
          personalityPrompt: editingAgent.config.personalityPrompt ?? ''
        })
      } else {
        form.resetFields()
        form.setFieldsValue({
        permissionMode: 'auto', maxTurns: 50, desktopEnabled: false, mobileEnabled: false, isDefault: false,
        iconName: '', iconColor: '', iconBgColor: '', group: '', maxSessionQueries: undefined, disallowedTools: undefined,
        behaviorProfile: cloneBehaviorProfile(), personalityTemplateName: '', personalityTemplateVersion: 0, personalityPrompt: ''
        })
      }
    }
  }, [open, editingAgent, form])

  const selectIcon = (name: string) => {
    const current = form.getFieldValue('iconName') as FormValues['iconName']
    if (current === name) {
      form.setFieldsValue({ iconName: '' })
      return
    }
    const opt = AGENT_ICON_OPTIONS.find((o) => o.name === name)
    const currentColor = form.getFieldValue('iconColor') as FormValues['iconColor'] | undefined
    const currentBg = form.getFieldValue('iconBgColor') as FormValues['iconBgColor'] | undefined
    form.setFieldsValue({
      iconName: name,
      iconColor: currentColor ?? opt?.defaultColor ?? '',
      iconBgColor: currentBg ?? (opt ? lightenHex(opt.defaultColor) : '')
    })
  }

  const handleSubmit = async () => {
    const v = await form.validateFields()
    const config: AgentConfig = {
      systemPrompt: v.systemPrompt,
      permissionMode: v.permissionMode,
      maxTurns: v.maxTurns,
      maxSessionQueries: v.maxSessionQueries ?? undefined,
      // undefined（新建未触碰）→ 不下发 key（hub 视为未配置）；编辑态清空后 antd Select 值为 []，
      // 必须显式发空数组——hub applyUpdateConfig 对 absent key 不变更，丢 key 会让清空静默失效
      disallowedTools: v.disallowedTools ?? undefined,
      title: v.titleZh ? { zh: v.titleZh, ...(v.titleEn ? { en: v.titleEn } : {}) } : undefined,
      description: v.descriptionZh
        ? { zh: v.descriptionZh, ...(v.descriptionEn ? { en: v.descriptionEn } : {}) }
        : undefined,
      iconName: v.iconName || undefined,
      iconColor: v.iconColor || undefined,
      iconBgColor: v.iconBgColor || undefined,
      group: v.group || '',
      behaviorProfile: v.behaviorProfile,
      personalityTemplateName: v.personalityTemplateName || '',
      personalityTemplateVersion: v.personalityTemplateVersion || 0,
      personalityPrompt: v.personalityPrompt || ''
    }

    if (editingAgent) {
      await updateAgent.mutateAsync({
        name: editingAgent.name,
        data: { config, desktopEnabled: v.desktopEnabled, mobileEnabled: v.mobileEnabled, isDefault: v.isDefault }
      })
    } else {
      await createAgent.mutateAsync({
        name: v.name,
        config,
        desktopEnabled: v.desktopEnabled,
        mobileEnabled: v.mobileEnabled,
        isDefault: v.isDefault
      })
    }
    onClose()
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable={false}
      width={720}
      styles={{ body: { padding: 0 } }}
      title={null}
      destroyOnHidden
    >
      <div className={styles.head}>
        <div className={styles.title}>{editingAgent ? '编辑代理' : '新建代理'}</div>
        <button type="button" className={styles.closeBtn} onClick={onClose}>
          <XIcon size={18} />
        </button>
      </div>

      <Form form={form} layout="vertical" className={styles.body} requiredMark={false}>
        {/* 基本信息 */}
        <div className={styles.section}>基本信息</div>
        <div className={styles.row}>
          <Form.Item label="代理标识" name="name" rules={agentIdentifierFormRules('代理标识')}>
            <Input placeholder="例如: general, code-review" disabled={!!editingAgent} />
          </Form.Item>
          <Form.Item label="权限模式" name="permissionMode">
            <Select options={[
              { label: 'Auto', value: 'auto' },
              { label: 'Plan', value: 'plan' },
              { label: 'Bypass', value: 'bypassPermissions' }
            ]} />
          </Form.Item>
        </div>
        <Form.Item label="分组" name="group">
          <AutoComplete
            options={groupOptions}
            placeholder="选择或输入新分组"
            allowClear
            suffix={<CaretDownIcon size={14} />}
            showSearch={{
              filterOption: (inputValue, option) => {
                const label = (option?.label ?? '').toLowerCase()
                return label.includes(inputValue.toLowerCase())
              },
            }}
          />
        </Form.Item>

        {/* 显示设置 */}
        <div className={styles.section} style={{ marginTop: 20 }}>显示设置</div>
        <div className={styles.row}>
          <Form.Item label="中文标题" name="titleZh">
            <Input placeholder="给代理起个中文名" />
          </Form.Item>
          <Form.Item label="English Title" name="titleEn">
            <Input placeholder="Agent display name" />
          </Form.Item>
        </div>
        <div className={styles.row}>
          <Form.Item label="中文描述" name="descriptionZh">
            <Input placeholder="简要描述用途" />
          </Form.Item>
          <Form.Item label="English Description" name="descriptionEn">
            <Input placeholder="Brief description" />
          </Form.Item>
        </div>

        {/* Icon picker */}
        <Form.Item label="图标" name="iconName">
          <div className={styles.iconGrid}>
            {AGENT_ICON_OPTIONS.map((opt) => {
              const IconCmp = getIconComponent(opt.name)
              const active = iconName === opt.name
              const activeColor = iconColor || opt.defaultColor
              const activeBg = iconBgColor || lightenHex(activeColor, 0.12)
              return (
                <button
                  key={opt.name}
                  type="button"
                  className={`${styles.iconCell} ${active ? styles.iconCellActive : ''}`}
                  style={active ? {
                    backgroundColor: activeBg,
                    borderColor: activeColor
                  } : {}}
                  title={opt.label}
                  onClick={(e) => { e.stopPropagation(); selectIcon(opt.name) }}
                >
                  <IconCmp size={18} weight="duotone" color={active ? activeColor : '#9CA3AF'} />
                </button>
              )
            })}
          </div>
        </Form.Item>

        {/* Color pickers */}
        <div className={styles.row}>
          <Form.Item label="图标颜色" name="iconColor">
            <div>
              <div className={styles.colorStrip}>
                {PRESET_COLORS.map((c) => (
                  <span
                    key={c}
                    className={`${styles.colorDot} ${iconColor === c ? styles.colorDotOn : ''}`}
                    style={{ backgroundColor: c }}
                    onClick={(e) => { e.stopPropagation(); form.setFieldsValue({ iconColor: c }) }}
                  />
                ))}
              </div>
              <Input size="small" placeholder="#1A3A5C" value={iconColor}
                onChange={(e) => { form.setFieldsValue({ iconColor: e.target.value }); }} />
            </div>
          </Form.Item>
          <Form.Item label="背景颜色" name="iconBgColor">
            <div>
              <div className={styles.colorStrip}>
                  {PRESET_BG_COLORS.map((c) => (
                    <span
                      key={c}
                      className={`${styles.colorDot} ${iconBgColor === c ? styles.colorDotOn : ''}`}
                      style={{ backgroundColor: c }}
                      onClick={(e) => { e.stopPropagation(); form.setFieldsValue({ iconBgColor: c }) }}
                    />
                  ))}
              </div>
              <Input size="small" placeholder="#EBF0FF" value={form.getFieldValue('iconBgColor') as string}
                onChange={(e) => { form.setFieldsValue({ iconBgColor: e.target.value }); }} />
            </div>
          </Form.Item>
        </div>

        {/* 能力配置 */}
        <div className={styles.section} style={{ marginTop: 20 }}>能力配置</div>
        <Form.Item label="最大轮次" name="maxTurns">
          <InputNumber min={1} max={500} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label="会话查询数上限" name="maxSessionQueries" tooltip="控制单个会话内的最大查询次数，留空表示无限制">
          <InputNumber min={1} placeholder="无限制" style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item label="禁用工具" name="disallowedTools" tooltip="在允许范围基础上剔除的工具名黑名单；可填内置工具名或 mcp__服务器__工具 形式的 MCP 工具名">
          <Select mode="tags" open={false} tokenSeparators={[',']} placeholder="输入要禁用的工具名，回车添加" style={{ width: '100%' }} />
        </Form.Item>

        {/* Prompt-first 人格 */}
        <div className={styles.section} style={{ marginTop: 20 }}>人格</div>
        <div className={styles.personalityShell}>
          <div className={styles.personalityHead}>
            <div>
              <span className={styles.personalityLabel}>人格原稿</span>
              <span className={styles.personalityHint}>选用模板后仍可修改；保存时会固化当前原稿，不受模板未来版本影响。</span>
            </div>
            <span className={styles.personalityVersion}>{personalityTemplateVersion ? `SNAPSHOT v${personalityTemplateVersion}` : 'CUSTOM'}</span>
          </div>
          <div className={styles.personalityBody}>
            <Form.Item label="从人格库选用" name="personalityTemplateName">
              <Select
                showSearch={{ optionFilterProp: 'label' }} allowClear placeholder="选择人格，或直接撰写自定义原稿"
                options={personalities.filter((item) => item.enabled || item.name === personalityTemplateName).map((item) => ({
                  value: item.name, label: `${item.title} · v${item.currentVersion}`
                }))}
                onChange={(name?: string) => {
                  if (!name) {
                    form.setFieldsValue({ personalityTemplateName: '', personalityTemplateVersion: 0 })
                    return
                  }
                  const template = personalities.find((item) => item.name === name)
                  if (!template) return
                  form.setFieldsValue({
                    personalityTemplateName: template.name,
                    personalityTemplateVersion: template.currentVersion,
                    personalityPrompt: template.prompt,
                    ...(template.behaviorProfile
                      ? { behaviorProfile: cloneBehaviorProfile(template.behaviorProfile) }
                      : {})
                  })
                }}
              />
            </Form.Item>
            <Form.Item name="personalityTemplateVersion" hidden><InputNumber /></Form.Item>
            <Form.Item label="提示词原稿" name="personalityPrompt" rules={[{ max: 40000 }]}>
              <Input.TextArea
                className={styles.personalityPrompt} rows={9} maxLength={40000} showCount
                placeholder="写清楚价值排序、决策习惯、沟通方式、越级条件和人格盲点。"
              />
            </Form.Item>
            <div className={styles.personalityFoot}>
              {personalityModified ? '当前原稿已在模板基础上修改；本 Agent 将保存这份独立快照。' : '人格影响判断与表达，不会授予工具、数据、通信或越级权限。'}
            </div>
          </div>
        </div>

        <div style={{ marginTop: 12 }}>
          <Form.Item name="behaviorProfile" noStyle>
            <BehaviorProfileEditor />
          </Form.Item>
        </div>

        {/* 系统提示词 */}
        <div className={styles.section} style={{ marginTop: 20 }}>系统提示词</div>
        <Form.Item name="systemPrompt">
          <Input.TextArea placeholder="定义代理的行为和指令..." rows={6} maxLength={20000} showCount />
        </Form.Item>

        {/* Toggles */}
        <div style={{ marginTop: 20 }}>
          <Form.Item name="desktopEnabled" valuePropName="checked" style={{ marginBottom: 0 }}>
            <ToggleItem
              title="桌面端代理"
              desc="桌面客户端加载此代理"
            />
          </Form.Item>
          <Form.Item name="mobileEnabled" valuePropName="checked" style={{ marginBottom: 0 }}>
            <ToggleItem
              title="手机端代理"
              desc="手机端加载此代理（预留）"
            />
          </Form.Item>
          <Form.Item name="isDefault" valuePropName="checked" style={{ marginBottom: 0 }}>
            <ToggleItem
              title="设为默认"
              desc="在新对话中自动预选此代理"
            />
          </Form.Item>
        </div>
      </Form>

      <div className={styles.foot}>
        <Button onClick={onClose}>取消</Button>
        <PrimaryButton onClick={handleSubmit} loading={submitting}>
          {editingAgent ? '保存更新' : '创建代理'}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
