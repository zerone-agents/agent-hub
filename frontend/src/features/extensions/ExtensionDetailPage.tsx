// H7.0 扩展注册中心 · 扩展详情页。
//
// 展示单个扩展的基本信息与版本列表；每个版本可展开查看 manifest 摘要
// （声明计数、UI 插槽）与权限清单，并可查看该版本完整 manifest JSON。
import { useState } from 'react'
import { Alert, Button, Descriptions, Drawer, Empty, Space, Table, Tag } from 'antd'
import { ArrowLeftIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useNavigate, useParams } from 'react-router'
import { parseApiError } from '@/api/client'
import { useExtensionDetail, useExtensionVersion } from '@/queries/useExtensionRegistry'
import ExtensionLifecyclePanel from './ExtensionLifecyclePanel'
import ExtensionGrantsPanel from './ExtensionGrantsPanel'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  page: css`
    width: 100%;
    max-width: 1200px;
    margin: 0 auto;
  `,
  header: css`
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
  `,
  title: css`
    margin: 0;
    color: ${t.text};
    font-size: ${t.text2xl};
    font-weight: 650;
  `,
  manifest: css`
    white-space: pre-wrap;
    word-break: break-all;
    font-family: monospace;
    font-size: 12px;
    background: var(--muted);
    padding: 12px;
    border-radius: ${t.radiusSm}px;
    max-height: 60vh;
    overflow: auto;
  `
}))

export default function ExtensionDetailPage() {
  const { styles } = useStyles()
  const navigate = useNavigate()
  const params = useParams()
  const id = Number(params.id)
  const { data, isLoading, error } = useExtensionDetail(Number.isFinite(id) && id > 0 ? id : undefined)
  const [viewVersion, setViewVersion] = useState<string | undefined>()
  const versionQuery = useExtensionVersion(id, viewVersion)

  if (error) {
    return (
      <div className={styles.page}>
        <Alert type="error" showIcon message="加载扩展详情失败" description={parseApiError(error)} />
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <Button icon={<ArrowLeftIcon size={16} />} onClick={() => navigate('/extensions')}>
          返回
        </Button>
        <h1 className={styles.title}>{data?.name ?? '扩展详情'}</h1>
        {data && <Tag color={data.status === 'active' ? 'green' : 'default'}>{data.status}</Tag>}
      </div>

      {data && (
        <>
          <ExtensionLifecyclePanel id={id} data={data} />
          {/* H7.4 权限与隔离：授权列表 / 撤销 / 调用审计 */}
          <ExtensionGrantsPanel extensionId={id} />
          <Descriptions column={2} style={{ marginBottom: 24 }}>
            <Descriptions.Item label="显示名">{data.displayName || '—'}</Descriptions.Item>
            <Descriptions.Item label="来源">{data.source}</Descriptions.Item>
            <Descriptions.Item label="描述" span={2}>
              {data.description || '—'}
            </Descriptions.Item>
          </Descriptions>
        </>
      )}

      <Table
        rowKey="id"
        loading={isLoading}
        dataSource={data?.versions ?? []}
        pagination={false}
        expandable={{
          expandedRowRender: (record) => (
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <div>
                <strong>Manifest 摘要</strong>
                <div style={{ marginTop: 6, color: t.textSecondary }}>
                  状态模式 {record.manifestSummary.stateSchemaCount} · 事件{' '}
                  {record.manifestSummary.eventCount} · 工具 {record.manifestSummary.toolCount} · 关系{' '}
                  {record.manifestSummary.relationCount} · 提示注入{' '}
                  {record.manifestSummary.promptInjectionCount}
                </div>
                {record.manifestSummary.slots && record.manifestSummary.slots.length > 0 && (
                  <div style={{ marginTop: 6 }}>
                    UI 插槽：
                    {record.manifestSummary.slots.map((slot) => (
                      <Tag key={slot} style={{ marginInlineEnd: 4 }}>
                        {slot}
                      </Tag>
                    ))}
                  </div>
                )}
              </div>
              <div>
                <strong>权限清单</strong>
                {record.permissions.length === 0 ? (
                  <div style={{ marginTop: 6, color: t.textTertiary }}>未声明权限</div>
                ) : (
                  <Table
                    rowKey={(r) => `${r.permission}:${r.scope}`}
                    size="small"
                    style={{ marginTop: 6 }}
                    dataSource={record.permissions}
                    pagination={false}
                    columns={[
                      { title: '权限类', dataIndex: 'permission', width: 120 },
                      { title: '范围', dataIndex: 'scope' },
                      {
                        title: '操作',
                        dataIndex: 'actions',
                        render: (actions: string[]) => actions.map((a) => <Tag key={a}>{a}</Tag>)
                      }
                    ]}
                  />
                )}
              </div>
            </Space>
          )
        }}
        columns={[
          { title: '版本', dataIndex: 'version', width: 140 },
          {
            title: '内容哈希',
            dataIndex: 'contentHash',
            render: (v: string) => (
              <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v.slice(0, 12)}…</span>
            )
          },
          { title: '变更说明', dataIndex: 'changelog', render: (v: string) => v || '—' },
          { title: '创建人', dataIndex: 'createdBy', width: 120, render: (v: string) => v || '—' },
          {
            title: '创建时间',
            dataIndex: 'createdAt',
            width: 180,
            render: (v: string) => (v ? new Date(v).toLocaleString() : '—')
          },
          {
            title: '操作',
            key: 'actions',
            width: 160,
            render: (_, record) => (
              <Button size="small" onClick={() => setViewVersion(record.version)}>
                完整 Manifest
              </Button>
            )
          }
        ]}
        locale={{ emptyText: <Empty description="暂无版本" /> }}
      />

      <Drawer
        title={viewVersion ? `版本 ${viewVersion} · 完整 Manifest` : '完整 Manifest'}
        open={!!viewVersion}
        onClose={() => setViewVersion(undefined)}
        width={640}
      >
        {versionQuery.isLoading && <div>加载中…</div>}
        {versionQuery.data && (
          <pre className={styles.manifest}>
            {JSON.stringify(JSON.parse(versionQuery.data.manifest), null, 2)}
          </pre>
        )}
        {versionQuery.error && (
          <Alert type="error" showIcon message={parseApiError(versionQuery.error)} />
        )}
      </Drawer>
    </div>
  )
}
