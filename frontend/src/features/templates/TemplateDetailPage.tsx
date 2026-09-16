// H7.3 模板库 · 模板详情页。
//
// 展示模板元信息、版本列表与 spec 摘要（将创建多少 agent/群组/关系/
// 工作流/状态 Schema、扩展依赖数量），并提供「安装」入口进入三步安装
// 向导，以及「导出当前配置为模板」。
import { useState } from 'react'
import { Alert, Card, Descriptions, Empty, Input, Modal, Space, Table, Tag, message } from 'antd'
import { ArrowLeftIcon, ExportIcon, RocketLaunchIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useNavigate, useParams } from 'react-router'
import { parseApiError } from '@/api/client'
import type { TemplateSpec } from '@/api/templates'
import { parseJsonSafe } from '@/utils/format'
import {
  useExportTemplate,
  useRegisterTemplate,
  useTemplateDetail,
  useTemplateVersion
} from '@/queries/useTemplates'
import PrimaryButton from '@/components/PrimaryButton'
import { tokens as t } from '@/styles/tokens'
import TemplateInstallWizard from './TemplateInstallWizard'

const CATEGORY_LABELS: Record<string, { label: string; color: string }> = {
  team: { label: '团队', color: 'blue' },
  game: { label: '游戏', color: 'purple' },
  general: { label: '通用', color: 'default' }
}

const SUMMARY_COLUMNS = [
  { key: 'agents', label: 'Agent' },
  { key: 'personalityTemplates', label: '人格模板' },
  { key: 'groups', label: '群组' },
  { key: 'relations', label: '关系' },
  { key: 'workflows', label: '工作流' },
  { key: 'stateSchemas', label: '状态 Schema' },
  { key: 'extensionDeps', label: '扩展依赖' },
  { key: 'sampleData', label: '种子数据' }
] as const

const useStyles = createStyles(({ css }) => ({
  page: css`
    width: 100%;
    max-width: 1200px;
    margin: 0 auto;
  `,
  header: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 20px;
  `,
  title: css`
    margin: 0;
    color: ${t.text};
    font-size: ${t.text2xl};
    font-weight: 650;
  `,
  subtitle: css`
    margin: 5px 0 0;
    color: ${t.textTertiary};
    font-size: ${t.textBase};
  `,
  summaryRow: css`
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin: 16px 0;
  `,
  back: css`
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-bottom: 12px;
    color: ${t.textTertiary};
    cursor: pointer;
    &:hover {
      color: ${t.text};
    }
  `
}))

export default function TemplateDetailPage() {
  const { styles } = useStyles()
  const navigate = useNavigate()
  const params = useParams<{ id: string }>()
  const id = Number(params.id)
  const { data: detail, isLoading, error } = useTemplateDetail(Number.isFinite(id) ? id : undefined)
  const [selectedVersion, setSelectedVersion] = useState<string>()
  const version = selectedVersion ?? detail?.versions[0]?.version
  const { data: versionDetail } = useTemplateVersion(id, version)
  const [installOpen, setInstallOpen] = useState(false)
  const [exportOpen, setExportOpen] = useState(false)
  const [exportedJson, setExportedJson] = useState('')
  const doExport = useExportTemplate(id)
  const doRegister = useRegisterTemplate()

  if (!Number.isFinite(id) || error) {
    return (
      <div className={styles.page}>
        <Empty description={error ? parseApiError(error) : '模板不存在'} />
      </div>
    )
  }
  if (isLoading || !detail) {
    return <div className={styles.page}>加载中…</div>
  }

  // 不能裸 JSON.parse：spec 内容坏了会在 render 体内抛错被根 ErrorBoundary
  // 接住 → 整页白屏。解析失败退化成 undefined，页面其余部分照常渲染。
  const spec = parseJsonSafe<TemplateSpec>(versionDetail?.spec)

  const handleExport = async () => {
    try {
      const spec = await doExport.mutateAsync({})
      setExportedJson(JSON.stringify(spec, null, 2))
      setExportOpen(true)
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  const handleRegisterExported = async () => {
    try {
      const parsed = JSON.parse(exportedJson) as TemplateSpec
      const name = window.prompt('新模板名称（DNS 式，如 io.zerone.my-team）')
      if (!name) return
      const version = window.prompt('版本号（如 1.0.0）') || '1.0.0'
      const result = await doRegister.mutateAsync({
        name,
        displayName: name,
        description: `从当前配置导出（来源模板 ${detail.name}）`,
        category: detail.category,
        version,
        spec: parsed
      })
      message.success(`模板 ${result.template.name}@${result.version.version} 注册成功`)
      setExportOpen(false)
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  const latestSummary = detail.versions[0]?.summary

  return (
    <div className={styles.page}>
      <span className={styles.back} onClick={() => navigate('/templates')}>
        <ArrowLeftIcon size={14} /> 返回模板库
      </span>
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>
            {detail.displayName || detail.name}
            <Tag style={{ marginLeft: 8 }} color={CATEGORY_LABELS[detail.category]?.color}>
              {CATEGORY_LABELS[detail.category]?.label ?? detail.category}
            </Tag>
            {detail.source === 'seed' && <Tag>内置种子</Tag>}
          </h1>
          <p className={styles.subtitle}>
            {detail.description || '（无描述）'}　<span>{detail.name}</span>
          </p>
        </div>
        <Space>
          <PrimaryButton icon={<ExportIcon size={16} />} onClick={handleExport} loading={doExport.isPending}>
            导出当前配置
          </PrimaryButton>
          <PrimaryButton
            icon={<RocketLaunchIcon size={16} />}
            onClick={() => setInstallOpen(true)}
          >
            安装
          </PrimaryButton>
        </Space>
      </div>

      {latestSummary && (
        <div className={styles.summaryRow}>
          {SUMMARY_COLUMNS.map((col) => (
            <Tag key={col.key} color={latestSummary[col.key] > 0 ? 'blue' : 'default'}>
              {col.label} × {latestSummary[col.key]}
            </Tag>
          ))}
        </div>
      )}

      {spec?.extensionDeps && spec.extensionDeps.length > 0 && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="扩展依赖"
          description={spec.extensionDeps
            .map((d) => `${d.name}${d.versionRange ? `@${d.versionRange}` : ''}`)
            .join('、')}
        />
      )}

      <Card title="版本" size="small">
        <Table
          rowKey="id"
          size="small"
          pagination={false}
          dataSource={detail.versions}
          onRow={(record) => ({
            onClick: () => setSelectedVersion(record.version),
            style: { cursor: 'pointer' }
          })}
          columns={[
            {
              title: '版本',
              dataIndex: 'version',
              render: (v: string) => <Tag color={v === version ? 'green' : 'default'}>v{v}</Tag>
            },
            { title: '内容哈希', dataIndex: 'contentHash', render: (v: string) => v.slice(0, 12) + '…' },
            { title: '创建者', dataIndex: 'createdBy' },
            { title: '创建时间', dataIndex: 'createdAt' },
            {
              title: '内容摘要',
              render: (_, record) =>
                SUMMARY_COLUMNS.filter((c) => record.summary[c.key] > 0)
                  .map((c) => `${c.label}×${record.summary[c.key]}`)
                  .join('，') || '（空）'
            }
          ]}
        />
      </Card>

      {spec && (
        <Card title={`Spec 摘要（v${version}）`} size="small" style={{ marginTop: 16 }}>
          <Descriptions column={2} size="small">
            {spec.agents?.map((a) => (
              <Descriptions.Item key={a.name} label={`Agent · ${a.name}`}>
                {a.title || a.name}
                {a.modelRef ? `（模型引用 ${a.modelRef}）` : ''}
              </Descriptions.Item>
            ))}
            {spec.groups?.map((g) => (
              <Descriptions.Item key={g.name} label={`群组 · ${g.name}`}>
                成员 {g.memberRefs?.length ?? 0} 人{g.channels?.length ? `，频道 ${g.channels.join('、')}` : ''}
              </Descriptions.Item>
            ))}
            {spec.workflows?.map((w) => (
              <Descriptions.Item key={w.name} label={`工作流 · ${w.name}`}>
                {w.steps.length} 个步骤
              </Descriptions.Item>
            ))}
            {spec.stateSchemas?.map((ss) => (
              <Descriptions.Item key={`${ss.namespace}/${ss.name}`} label="状态 Schema">
                {ss.namespace}/{ss.name}@{ss.version}
              </Descriptions.Item>
            ))}
          </Descriptions>
        </Card>
      )}

      <Modal
        title="安装模板"
        open={installOpen}
        onCancel={() => setInstallOpen(false)}
        footer={null}
        width={860}
        destroyOnHidden
      >
        <TemplateInstallWizard
          templateId={detail.id}
          defaultVersion={version}
          onClose={() => setInstallOpen(false)}
        />
      </Modal>

      <Modal
        title="导出结果（可作为新模板注册）"
        open={exportOpen}
        onCancel={() => setExportOpen(false)}
        onOk={handleRegisterExported}
        okText="注册为新模板"
        confirmLoading={doRegister.isPending}
        width={720}
        destroyOnHidden
      >
        <Input.TextArea
          rows={16}
          value={exportedJson}
          onChange={(e) => setExportedJson(e.target.value)}
        />
      </Modal>
    </div>
  )
}
