// H7.0 扩展注册中心 · 扩展列表页。
//
// 展示租户内已注册扩展（名称/版本数/状态/来源），支持按状态与来源过滤、
// 注册新扩展（粘贴 manifest JSON，重复内容哈希幂等返回既有版本）。
import { useState } from 'react'
import { Alert, Input, Modal, Select, Table, Tag, Space, message } from 'antd'
import { PackageIcon, PlusIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { Link } from 'react-router'
import { parseApiError } from '@/api/client'
import { useExtensionList, useRegisterExtension } from '@/queries/useExtensionRegistry'
import PrimaryButton from '@/components/PrimaryButton'
import { tokens as t } from '@/styles/tokens'

const STATUS_LABELS: Record<string, { label: string; color: string }> = {
  active: { label: '启用', color: 'green' },
  disabled: { label: '停用', color: 'default' },
  draft: { label: '草稿', color: 'gold' }
}

const SOURCE_LABELS: Record<string, string> = {
  seed: '内置种子',
  registry: '注册中心',
  upload: '手动上传'
}

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
  toolbar: css`
    display: flex;
    gap: 12px;
    margin-bottom: 16px;
  `
}))

export default function ExtensionListPage() {
  const { styles } = useStyles()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [status, setStatus] = useState<string>()
  const [source, setSource] = useState<string>()
  const { data, isLoading, error } = useExtensionList({ status, source, page, pageSize })
  const register = useRegisterExtension()
  const [registerOpen, setRegisterOpen] = useState(false)
  const [manifest, setManifest] = useState('')

  const handleRegister = async () => {
    try {
      const result = await register.mutateAsync({ manifest })
      message.success(
        result.alreadyExisted
          ? `扩展 ${result.extension.name}@${result.version.version} 已存在，幂等返回`
          : `扩展 ${result.extension.name}@${result.version.version} 注册成功`
      )
      setRegisterOpen(false)
      setManifest('')
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>
            <PackageIcon size={24} style={{ verticalAlign: '-4px', marginRight: 8 }} />
            扩展
          </h1>
          <p className={styles.subtitle}>扩展注册中心：注册、版本与权限声明管理（H7.0）</p>
        </div>
        <PrimaryButton icon={<PlusIcon size={16} />} onClick={() => { setRegisterOpen(true); }}>
          注册扩展
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 140 }}
          value={status}
          onChange={(v) => {
            setStatus(v)
            setPage(1)
          }}
          options={Object.entries(STATUS_LABELS).map(([value, s]) => ({ value, label: s.label }))}
        />
        <Select
          allowClear
          placeholder="来源"
          style={{ width: 140 }}
          value={source}
          onChange={(v) => {
            setSource(v)
            setPage(1)
          }}
          options={Object.entries(SOURCE_LABELS).map(([value, label]) => ({ value, label }))}
        />
      </div>

      {error && <Alert type="error" showIcon message="加载扩展列表失败" description={parseApiError(error)} />}

      <Table
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        pagination={{
          current: page,
          pageSize,
          total: data?.total ?? 0,
          showSizeChanger: true,
          onChange: (p, ps) => {
            setPage(p)
            setPageSize(ps)
          }
        }}
        columns={[
          {
            title: '名称',
            dataIndex: 'name',
            render: (_, record) => (
              <Space direction="vertical" size={0}>
                {/* 用 Link 而不是 <a onClick>：后者没有 href，无法键盘聚焦、
                    无法 cmd+点击新标签打开，也无右键「复制链接地址」（P2-22）。 */}
                <Link to={`/extensions/${record.id}`}>{record.name}</Link>
                <span style={{ color: t.textTertiary, fontSize: 12 }}>
                  {record.displayName || '—'}
                </span>
              </Space>
            )
          },
          {
            title: '最新版本',
            dataIndex: 'latestVersion',
            width: 120,
            render: (v: string) => v || '—'
          },
          {
            title: '版本数',
            dataIndex: 'versionCount',
            width: 90
          },
          {
            title: '安装状态',
            key: 'installed',
            width: 130,
            render: (_, record) =>
              record.installed ? (
                <Tag color={record.installedStatus === 'enabled' ? 'green' : 'default'}>
                  已安装 {record.installedVersion}
                  {record.installedStatus === 'disabled' ? '（已停用）' : ''}
                </Tag>
              ) : (
                <Tag>未安装</Tag>
              )
          },
          {
            title: '状态',
            dataIndex: 'status',
            width: 100,
            render: (v: string) => {
              const s = STATUS_LABELS[v] ?? { label: v, color: 'default' }
              return <Tag color={s.color}>{s.label}</Tag>
            }
          },
          {
            title: '来源',
            dataIndex: 'source',
            width: 110,
            render: (v: string) => SOURCE_LABELS[v] ?? v
          },
          {
            title: '注册时间',
            dataIndex: 'createdAt',
            width: 180,
            render: (v: string) => (v ? new Date(v).toLocaleString() : '—')
          }
        ]}
      />

      <Modal
        title="注册扩展"
        open={registerOpen}
        onCancel={() => { setRegisterOpen(false); }}
        onOk={handleRegister}
        confirmLoading={register.isPending}
        okText="注册"
        cancelText="取消"
        width={640}
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message="请求体即扩展 manifest JSON（agenthub.extension/v1alpha1），重复内容将幂等返回既有版本。"
        />
        <Input.TextArea
          rows={14}
          placeholder='{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.example",...}'
          value={manifest}
          onChange={(e) => { setManifest(e.target.value); }}
        />
      </Modal>
    </div>
  )
}
