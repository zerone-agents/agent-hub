// H7.3 模板库 · 安装向导（三步）。
//
// ① 选择安装段（agents/groups/relations/workflows/stateSchemas/sampleData…）
// ② 映射模型（modelRef → 实际模型 ID）+ 名称前缀 + 冲突策略 + force
// ③ 预览执行计划（将创建什么/冲突/缺失扩展依赖）→ 确认安装 → 结果页
// （成功清单 / skipped / 冲突）。幂等键自动生成（可修改），重试安全。
import { useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Checkbox,
  Descriptions,
  Input,
  Result,
  Space,
  Steps,
  Table,
  Tag,
  message
} from 'antd'
import { createStyles } from 'antd-style'
import { parseApiError } from '@/api/client'
import { parseJsonSafe } from '@/utils/format'
import type {
  TemplateInstallPlan,
  TemplateInstallResult,
  TemplateSpec
} from '@/api/templates'
import {
  useInstallTemplate,
  usePreviewTemplate,
  useTemplateVersion
} from '@/queries/useTemplates'
import PrimaryButton from '@/components/PrimaryButton'
import { tokens as t } from '@/styles/tokens'

const ALL_SECTIONS = [
  { key: 'personalityTemplates', label: '人格模板' },
  { key: 'agents', label: 'Agents' },
  { key: 'groups', label: '群组（含频道/成员）' },
  { key: 'relations', label: '关系' },
  { key: 'workflows', label: '工作流' },
  { key: 'stateSchemas', label: '状态 Schema' },
  { key: 'sampleData', label: '种子数据（需目标运行）' }
]

const SECTION_DEPENDENCIES: Record<string, string[]> = {
  relations: ['agents']
}

const useStyles = createStyles(({ css }) => ({
  stepBody: css`
    margin: 20px 0;
    min-height: 260px;
  `,
  footer: css`
    display: flex;
    justify-content: space-between;
    margin-top: 16px;
  `,
  modelRefRow: css`
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  `,
  planSection: css`
    margin-bottom: 8px;
    font-weight: 600;
    color: ${t.text};
  `
}))

interface WizardProps {
  templateId: number
  defaultVersion?: string
  onClose: () => void
}

export default function TemplateInstallWizard({ templateId, defaultVersion, onClose }: WizardProps) {
  const { styles } = useStyles()
  const [step, setStep] = useState(0)
  const [version] = useState<string | undefined>(defaultVersion)
  const { data: versionDetail } = useTemplateVersion(templateId, version)
  // 裸 JSON.parse 放在 render 体内有两个问题：spec 内容坏了会在渲染时抛错
  // 被根 ErrorBoundary 接住（整页白屏），而且每次 render 都重解析一遍。
  // 收进 useMemo + parseJsonSafe（解析失败退化成 undefined，页面照常渲染）。
  const spec = useMemo(
    () => parseJsonSafe<TemplateSpec>(versionDetail?.spec),
    [versionDetail?.spec]
  )

  // ① sections
  const [sections, setSections] = useState<string[]>(ALL_SECTIONS.map((s) => s.key))
  // ② mapping
  const [modelRefs, setModelRefs] = useState<Record<string, string>>({})
  const [namePrefix, setNamePrefix] = useState('')
  const [strategy, setStrategy] = useState<'fail' | 'rename'>('fail')
  const [force, setForce] = useState(false)
  const [targetRunId, setTargetRunId] = useState('')
  const [idempotencyKey, setIdempotencyKey] = useState(
    () => `tpl-${templateId}-${Date.now()}`
  )
  // ③ plan / result
  const [plan, setPlan] = useState<TemplateInstallPlan | null>(null)
  const [result, setResult] = useState<TemplateInstallResult | null>(null)
  const preview = usePreviewTemplate(templateId)
  const install = useInstallTemplate(templateId)

  const specModelRefs = useMemo(() => {
    const refs = new Set<string>()
    spec?.agents?.forEach((a) => a.modelRef && refs.add(a.modelRef))
    return Array.from(refs)
  }, [spec])

  const toggleSection = (key: string, checked: boolean) => {
    let next = checked ? [...sections, key] : sections.filter((s) => s !== key)
    // 段依赖：取消 agents 时级联取消依赖它的段
    if (!checked) {
      for (const [section, deps] of Object.entries(SECTION_DEPENDENCIES)) {
        if (deps.includes(key) && next.includes(section)) {
          next = next.filter((s) => s !== section)
          message.info(`「${ALL_SECTIONS.find((s) => s.key === section)?.label}」依赖被取消的段，已一并取消`)
        }
      }
    } else {
      // 勾选段时自动勾选其依赖
      for (const [section, deps] of Object.entries(SECTION_DEPENDENCIES)) {
        if (section === key) {
          next = Array.from(new Set([...next, ...deps]))
        }
      }
    }
    setSections(next)
  }

  const buildRequest = () => ({
    version,
    mapping: { modelRefs, namePrefix },
    sections,
    strategy,
    force
  })

  const handlePreview = async () => {
    try {
      const p = await preview.mutateAsync(buildRequest())
      setPlan(p)
      setStep(2)
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  const handleInstall = async () => {
    try {
      const r = await install.mutateAsync({
        ...buildRequest(),
        idempotencyKey,
        targetRunId: targetRunId || undefined
      })
      setResult(r)
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  const groupedPlanItems = useMemo(() => {
    if (!plan) return []
    const groups = new Map<string, typeof plan.items>()
    for (const item of plan.items) {
      const list = groups.get(item.section) ?? []
      list.push(item)
      groups.set(item.section, list)
    }
    return Array.from(groups.entries())
  }, [plan])

  // 结果页
  if (result) {
    return (
      <Result
        status="success"
        title={result.idempotentReplay ? '幂等重放：返回首次安装结果' : '安装成功'}
        subTitle={result.plan ? `模板 ${result.plan.templateName}@${result.plan.version}` : undefined}
        extra={[
          <PrimaryButton key="close" onClick={onClose}>
            完成
          </PrimaryButton>
        ]}
      >
        {Object.entries(result.created).map(([section, names]) => (
          <div key={section} style={{ marginBottom: 12, textAlign: 'left' }}>
            <Tag color="green">{ALL_SECTIONS.find((s) => s.key === section)?.label ?? section}</Tag>
            {names.join('、')}
          </div>
        ))}
        {result.skipped?.map((s) => (
          <Alert key={s} type="warning" showIcon message={s} style={{ marginTop: 8, textAlign: 'left' }} />
        ))}
      </Result>
    )
  }

  return (
    <div>
      <Steps
        current={step}
        items={[{ title: '选择安装段' }, { title: '映射与选项' }, { title: '预览并确认' }]}
      />

      <div className={styles.stepBody}>
        {step === 0 && (
          <Space direction="vertical">
            {ALL_SECTIONS.map((s) => (
              <Checkbox
                key={s.key}
                checked={sections.includes(s.key)}
                onChange={(e) => { toggleSection(s.key, e.target.checked); }}
              >
                {s.label}
              </Checkbox>
            ))}
            {(!spec?.agents || spec.agents.length === 0) && (
              <Alert type="info" showIcon message="该模板不包含 agents 段，可跳过映射步骤。" />
            )}
          </Space>
        )}

        {step === 1 && (
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {specModelRefs.length > 0 && (
              <>
                <div className={styles.planSection}>模型映射（模板 modelRef → 实际模型 ID）</div>
                {specModelRefs.map((ref) => (
                  <div key={ref} className={styles.modelRefRow}>
                    <Tag>{ref}</Tag>
                    <span>→</span>
                    <Input
                      style={{ width: 320 }}
                      placeholder="实际模型 ID，如 gpt-4o"
                      value={modelRefs[ref] ?? ''}
                      onChange={(e) => { setModelRefs((prev) => ({ ...prev, [ref]: e.target.value })); }}
                    />
                  </div>
                ))}
              </>
            )}
            <div>
              <div className={styles.planSection}>名称前缀（可选，避免命名冲突）</div>
              <Input
                style={{ width: 320 }}
                placeholder="如 acme-"
                value={namePrefix}
                onChange={(e) => { setNamePrefix(e.target.value); }}
              />
            </div>
            <div>
              <div className={styles.planSection}>冲突策略</div>
              <Space>
                <Tag.CheckableTag checked={strategy === 'fail'} onChange={() => { setStrategy('fail'); }}>
                  fail（冲突即中止，返回 409）
                </Tag.CheckableTag>
                <Tag.CheckableTag checked={strategy === 'rename'} onChange={() => { setStrategy('rename'); }}>
                  rename（自动加 -2 后缀）
                </Tag.CheckableTag>
              </Space>
            </div>
            <Space size={16}>
              <Checkbox checked={force} onChange={(e) => { setForce(e.target.checked); }}>
                force（跳过扩展依赖缺失阻断）
              </Checkbox>
            </Space>
            {sections.includes('sampleData') && (
              <div>
                <div className={styles.planSection}>目标运行（sampleData 写入；留空则跳过）</div>
                <Input
                  style={{ width: 320 }}
                  placeholder="run id"
                  value={targetRunId}
                  onChange={(e) => { setTargetRunId(e.target.value); }}
                />
              </div>
            )}
          </Space>
        )}

        {step === 2 && plan && (
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {plan.conflicts.length > 0 && (
              <Alert
                type="error"
                showIcon
                message={`存在 ${plan.conflicts.length} 个命名冲突（fail 策略将中止安装）`}
                description={plan.conflicts.map((c) => `${c.resourceType} ${c.name}：${c.reason}`).join('；')}
              />
            )}
            {plan.missingExtensions && plan.missingExtensions.length > 0 && (
              <Alert
                type="warning"
                showIcon
                message="缺少已启用的扩展依赖"
                description={
                  <>
                    {plan.missingExtensions.join('；')}
                    {!force && <div>可返回上一步勾选 force 跳过依赖阻断。</div>}
                  </>
                }
              />
            )}
            {plan.renames && plan.renames.length > 0 && (
              <Alert
                type="info"
                showIcon
                message="rename 策略将自动改名"
                description={plan.renames.map((r) => `${r.from} → ${r.to}`).join('；')}
              />
            )}
            {groupedPlanItems.map(([section, items]) => (
              <div key={section}>
                <div className={styles.planSection}>
                  {ALL_SECTIONS.find((s) => s.key === section)?.label ?? section}（{items.length}）
                </div>
                <Table
                  rowKey={(r) => `${r.kind}-${r.name}`}
                  size="small"
                  pagination={false}
                  dataSource={items}
                  columns={[
                    { title: '类型', dataIndex: 'kind', width: 160, render: (v: string) => <Tag>{v}</Tag> },
                    { title: '名称', dataIndex: 'name' },
                    { title: '说明', dataIndex: 'detail', render: (v?: string) => v ?? '—' }
                  ]}
                />
              </div>
            ))}
            <Descriptions column={1} size="small">
              <Descriptions.Item label="幂等键">
                <Input
                  style={{ width: 320 }}
                  value={idempotencyKey}
                  onChange={(e) => { setIdempotencyKey(e.target.value); }}
                />
              </Descriptions.Item>
            </Descriptions>
          </Space>
        )}
      </div>

      <div className={styles.footer}>
        <Button disabled={step === 0} onClick={() => { setStep(step - 1); }}>
          上一步
        </Button>
        {step < 1 && (
          <PrimaryButton
            onClick={() => {
              if (specModelRefs.length > 0 && specModelRefs.some((r) => !(modelRefs[r] ?? '').trim())) {
                message.warning('请为所有 modelRef 填写映射模型 ID')
                return
              }
              setStep(1)
            }}
          >
            下一步
          </PrimaryButton>
        )}
        {step === 1 && (
          <PrimaryButton onClick={handlePreview} loading={preview.isPending}>
            预览执行计划
          </PrimaryButton>
        )}
        {step === 2 && (
          <PrimaryButton onClick={handleInstall} loading={install.isPending} disabled={plan ? plan.conflicts.length > 0 && strategy === 'fail' : true}>
            确认安装
          </PrimaryButton>
        )}
      </div>
    </div>
  )
}
