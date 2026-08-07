import { useState, useMemo } from 'react'
import { Spin, Popconfirm, message } from 'antd'
import NameSearch from '@/components/NameSearch'
import { Plus, Star, Medal, UsersThree, ArrowDown, PencilSimple, Trash } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Skill } from '@/api/skills'
import { useSkills, useDeleteSkill } from '@/queries/useSkills'
import { skillApi } from '@/api/skills'
import type { ApiEnvelope } from '@/api/client'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
import EntityCard from '@/components/EntityCard'
import CardGrid from '@/components/CardGrid'
import SkillForm from './SkillForm'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @keyframes pageIn { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: translateY(0); } }
  `,
  pageHead: css`
    display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 24px;
    @media (max-width: 768px) { flex-direction: column; gap: 16px; }
  `,
  pageTitle: css`
    font-size: ${t.text3xl}; font-weight: 700; color: ${t.text}; letter-spacing: -0.03em; line-height: 1.15;
  `,
  pageSub: css`margin-top: 4px; font-size: ${t.textBase}; color: ${t.textTertiary};`,
  loadingWrap: css`display: flex; justify-content: center; padding: 80px 0;`,
  emptyState: css`text-align: center; padding: 80px 0;`,
  emptyTitle: css`font-size: ${t.textLg}; font-weight: 600; color: ${t.text}; margin-bottom: 6px;`,
  emptyDesc: css`color: ${t.textTertiary}; font-size: ${t.textSm};`,
  section: css`margin-bottom: 40px;`,
  sectionHeader: css`
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
  `,
  sectionTitle: css`display: flex; align-items: center; gap: 8px; color: ${t.text}; font-size: ${t.textBase}; font-weight: 600;`,
  sectionCount: css`
    display: inline-flex; align-items: center; justify-content: center;
    min-width: 24px; height: 24px; padding: 0 8px;
    background: ${t.inkSubtle}; color: ${t.ink}; border-radius: 12px;
    font-size: 12px; font-weight: 600;
  `,
  fileMeta: css`margin-top: 8px; font-size: 11px; color: ${t.textMuted};`,
  actBtn: css`
    width: 30px; height: 30px; display: flex; align-items: center; justify-content: center;
    border: none; background: transparent; border-radius: ${t.radiusSm}px;
    color: ${t.textMuted}; cursor: pointer; transition: all 0.15s;
    &:hover { background: ${t.inkSubtle}; color: ${t.ink}; }
    &:disabled { opacity: 0.3; cursor: not-allowed; }
  `,
  actBtnDanger: css`&:hover { background: rgba(220, 38, 38, 0.06); color: ${t.danger}; }`,
  toolbar: css`
    display: flex; justify-content: space-between; align-items: center;
    gap: 12px; margin-bottom: 16px;
  `,
}))

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export default function SkillListPage() {
  const { styles } = useStyles()
  const { data: skills = [], isLoading } = useSkills()
  const deleteSkill = useDeleteSkill()

  const [formOpen, setFormOpen] = useState(false)
  const [editingSkill, setEditingSkill] = useState<Skill | null>(null)

  // 搜索
  const [keywords, setKeywords] = useState('')

  // 按关键词过滤
  const filteredSkills = useMemo(() => {
    if (!keywords) return skills
    const kw = keywords.toLowerCase()
    return skills.filter((skill) => {
      const fields = [skill.title, skill.titleEn, skill.name, skill.description, skill.descriptionEn]
      return fields.some((f) => f.toLowerCase().includes(kw))
    })
  }, [skills, keywords])

  const expertSkills = filteredSkills.filter((s) => s.type === 'expert').sort((a, b) => a.name.localeCompare(b.name))
  const communitySkills = filteredSkills.filter((s) => s.type === 'community').sort((a, b) => a.name.localeCompare(b.name))

  const showEdit = (skill: Skill) => {
    setEditingSkill(skill)
    setFormOpen(true)
  }

  const handleDownload = async (skill: Skill) => {
    try {
      const res = await skillApi.download(skill.name)
      const body = res.data as ApiEnvelope<{ url?: string }>
      if (body.success && body.data?.url) {
        window.open(body.data.url, '_blank')
      }
    } catch {
      message.error('获取下载链接失败')
    }
  }

  const renderSkillCard = (skill: Skill) => (
    <EntityCard
      key={skill.name}
      icon={skill.name[0].toUpperCase()}
      title={skill.title || skill.titleEn || skill.name}
      subtitle={skill.name}
      headerExtra={
        <span
          style={{
            display: 'inline-block',
            padding: '1px 7px',
            borderRadius: 3,
            fontSize: 10,
            fontWeight: 600,
            background: skill.url ? 'rgba(5, 150, 105, 0.08)' : 'color-mix(in srgb, var(--foreground) 6%, transparent)',
            color: skill.url ? '#059669' : '#6b7b8a'
          }}
        >
          {skill.url ? '已上传' : '无文件'}
        </span>
      }
      description={skill.description || skill.descriptionEn || '暂无描述'}
      bodyExtra={
        skill.url ? (
          <div className={styles.fileMeta}>
            {formatFileSize(skill.fileSize)} · {skill.fileHash.slice(0, 8)}
          </div>
        ) : null
      }
      footerLeft={formatTime(skill.createdAt)}
      footerRight={
        <div style={{ display: 'flex', gap: 2 }}>
          <button
            type="button"
            className={styles.actBtn}
            title="下载"
            disabled={!skill.url}
            onClick={() => handleDownload(skill)}
          >
            <ArrowDown size={14} />
          </button>
          <button type="button" className={styles.actBtn} title="编辑" onClick={() => { showEdit(skill); }}>
            <PencilSimple size={14} />
          </button>
          <Popconfirm
            title="确认删除？"
            description={`删除 "${skill.name}"？此操作不可撤销。`}
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={() => { deleteSkill.mutate(skill.name); }}
          >
            <button type="button" className={`${styles.actBtn} ${styles.actBtnDanger}`} title="删除">
              <Trash size={14} />
            </button>
          </Popconfirm>
        </div>
      }
    />
  )

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>技能管理</div>
          <div className={styles.pageSub}>管理 AI 技能包，上传 zip 文件并关联到 Agent</div>
        </div>
        <PrimaryButton
          icon={<Plus size={16} weight="bold" />}
          onClick={() => { setEditingSkill(null); setFormOpen(true) }}
        >
          新建技能
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder="搜索技能名称"
            onSearch={setKeywords}
            realtime
          />
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}><Spin size="medium" /></div>
      ) : filteredSkills.length === 0 ? (
        <div className={styles.emptyState}>
          <div style={{ marginBottom: 20 }}><Star size={48} weight="thin" color={t.textMuted} /></div>
          <div className={styles.emptyTitle}>{keywords ? '未找到匹配的技能' : '暂无技能'}</div>
          <div className={styles.emptyDesc}>{keywords ? '请尝试其他关键词' : '创建您的第一个技能包以开始使用'}</div>
        </div>
      ) : (
        <>
          {expertSkills.length > 0 && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <div className={styles.sectionTitle}>
                  <Medal size={18} weight="duotone" />
                  专家技能
                </div>
                <span className={styles.sectionCount}>{expertSkills.length}</span>
              </div>
              <CardGrid>{expertSkills.map(renderSkillCard)}</CardGrid>
            </div>
          )}
          {communitySkills.length > 0 && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <div className={styles.sectionTitle}>
                  <UsersThree size={18} weight="duotone" />
                  社区技能
                </div>
                <span className={styles.sectionCount}>{communitySkills.length}</span>
              </div>
              <CardGrid>{communitySkills.map(renderSkillCard)}</CardGrid>
            </div>
          )}
        </>
      )}

      <SkillForm
        open={formOpen}
        editingSkill={editingSkill}
        onClose={() => { setFormOpen(false); }}
      />
    </div>
  )
}
