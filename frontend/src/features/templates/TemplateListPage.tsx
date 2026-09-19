// H7.3 模板库 · 模板列表页。
//
// 卡片 grid 展示模板（图标/名称/分类/描述/版本），支持分类过滤；
// 支持从 JSON 注册新模板（注册即严格校验，错误中文提示）。
import { useMemo, useState } from 'react'
import { Card, Empty, Input, Modal, Select, Space, Tag, message } from 'antd'
import { SquaresFourIcon, PlusIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useNavigate } from 'react-router'
import { parseApiError } from '@/api/client'
import type { TemplateSpec } from '@/api/templates'
import { useRegisterTemplate, useTemplateList } from '@/queries/useTemplates'
import PrimaryButton from '@/components/PrimaryButton'
import { tokens as t } from '@/styles/tokens'

const CATEGORY_LABELS: Record<string, { label: string; color: string }> = {
  team: { label: '团队', color: 'blue' },
  game: { label: '游戏', color: 'purple' },
  general: { label: '通用', color: 'default' }
}

const SOURCE_LABELS: Record<string, string> = {
  seed: '内置种子',
  user: '用户注册'
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
  `,
  grid: css`
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 16px;
  `,
  card: css`
    cursor: pointer;
    transition: box-shadow 0.2s ease;
    &:hover {
      box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
    }
  `,
  cardTitle: css`
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
    color: ${t.text};
    font-size: ${t.textBase};
    font-weight: 600;
  `,
  cardDesc: css`
    min-height: 40px;
    color: ${t.textTertiary};
    font-size: ${t.textSm};
  `,
  cardMeta: css`
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-top: 12px;
  `
}))

export default function TemplateListPage() {
  const { styles } = useStyles()
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [category, setCategory] = useState<string>()
  const { data, isLoading } = useTemplateList({ category, page, pageSize: 24 })
  const register = useRegisterTemplate()
  const [registerOpen, setRegisterOpen] = useState(false)
  const [registerJson, setRegisterJson] = useState('')

  const items = useMemo(() => data?.items ?? [], [data])

  const handleRegister = async () => {
    try {
      const parsed = JSON.parse(registerJson) as {
        name: string
        displayName?: string
        description?: string
        category?: string
        icon?: string
        version: string
        spec: TemplateSpec
      }
      if (!parsed.name || !parsed.version || !parsed.spec) {
        message.error('JSON 必须包含 name、version、spec 字段')
        return
      }
      const result = await register.mutateAsync({
        name: parsed.name,
        displayName: parsed.displayName ?? parsed.name,
        description: parsed.description ?? '',
        category: parsed.category ?? 'general',
        icon: parsed.icon,
        version: parsed.version,
        spec: parsed.spec
      })
      message.success(
        result.alreadyExisted
          ? `模板 ${result.template.name}@${result.version.version} 已存在，幂等返回`
          : `模板 ${result.template.name}@${result.version.version} 注册成功`
      )
      setRegisterOpen(false)
      setRegisterJson('')
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>
            <SquaresFourIcon size={24} style={{ verticalAlign: '-4px', marginRight: 8 }} />
            模板库
          </h1>
          <p className={styles.subtitle}>
            声明式模板：预览执行计划、映射模型、一键安装 Agent 团队/角色包（H7.3）
          </p>
        </div>
        <PrimaryButton icon={<PlusIcon size={16} />} onClick={() => { setRegisterOpen(true); }}>
          注册模板
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
        <Select
          allowClear
          placeholder="分类"
          style={{ width: 140 }}
          value={category}
          onChange={(v) => {
            setCategory(v)
            setPage(1)
          }}
          options={Object.entries(CATEGORY_LABELS).map(([value, c]) => ({ value, label: c.label }))}
        />
      </div>

      {items.length === 0 && !isLoading ? (
        <Empty description="暂无模板，点击右上角注册" />
      ) : (
        <div className={styles.grid}>
          {items.map((tpl) => (
            <Card
              key={tpl.id}
              className={styles.card}
              onClick={() => navigate(`/templates/${tpl.id}`)}
            >
              <div className={styles.cardTitle}>
                {tpl.displayName || tpl.name}
                <Tag color={CATEGORY_LABELS[tpl.category]?.color}>
                  {CATEGORY_LABELS[tpl.category]?.label ?? tpl.category}
                </Tag>
                {tpl.source === 'seed' && <Tag>{SOURCE_LABELS.seed}</Tag>}
              </div>
              <div className={styles.cardDesc}>{tpl.description || '（无描述）'}</div>
              <div className={styles.cardMeta}>
                <Space size={4}>
                  <Tag color="green">v{tpl.latestVersion}</Tag>
                  {tpl.versionCount > 1 && <Tag>{tpl.versionCount} 个版本</Tag>}
                </Space>
                <span style={{ color: t.textTertiary, fontSize: 12 }}>{tpl.name}</span>
              </div>
            </Card>
          ))}
        </div>
      )}

      <Modal
        title="注册模板"
        open={registerOpen}
        onCancel={() => { setRegisterOpen(false); }}
        onOk={handleRegister}
        confirmLoading={register.isPending}
        okText="注册"
        width={640}
        destroyOnHidden
      >
        <p style={{ color: t.textTertiary }}>
          粘贴模板 JSON（含 name、version、spec；spec 将严格校验，引用必须闭环）：
        </p>
        <Input.TextArea
          rows={12}
          value={registerJson}
          onChange={(e) => { setRegisterJson(e.target.value); }}
          placeholder='{"name":"io.zerone.example","version":"1.0.0","displayName":"…","category":"team","spec":{"agents":[…]}}'
        />
      </Modal>
    </div>
  )
}
