import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Spin, Popconfirm, message } from 'antd'
import NameSearch from '@/components/NameSearch'
import { PlusIcon, StarIcon, MedalIcon, UsersThreeIcon, ArrowDownIcon, PencilSimpleIcon, TrashIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Skill } from '@/api/skills'
import { useSkills, useDeleteSkill } from '@/queries/useSkills'
import { useCanWrite } from '@/hooks/useCanWrite'
import { skillApi } from '@/api/skills'
import type { ApiEnvelope } from '@/api/client'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'
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
    font-size: ${tk.text3xl}; font-weight: 700; color: ${tk.text}; letter-spacing: -0.03em; line-height: 1.15;
  `,
  pageSub: css`margin-top: 4px; font-size: ${tk.textBase}; color: ${tk.textTertiary};`,
  loadingWrap: css`display: flex; justify-content: center; padding: 80px 0;`,
  emptyState: css`text-align: center; padding: 80px 0;`,
  emptyTitle: css`font-size: ${tk.textLg}; font-weight: 600; color: ${tk.text}; margin-bottom: 6px;`,
  emptyDesc: css`color: ${tk.textTertiary}; font-size: ${tk.textSm};`,
  section: css`margin-bottom: 40px;`,
  sectionHeader: css`
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
  `,
  sectionTitle: css`display: flex; align-items: center; gap: 8px; color: ${tk.text}; font-size: ${tk.textBase}; font-weight: 600;`,
  sectionCount: css`
    display: inline-flex; align-items: center; justify-content: center;
    min-width: 24px; height: 24px; padding: 0 8px;
    background: ${tk.inkSubtle}; color: ${tk.ink}; border-radius: 12px;
    font-size: 12px; font-weight: 600;
  `,
  fileMeta: css`margin-top: 8px; font-size: 11px; color: ${tk.textMuted};`,
  actBtn: css`
    width: 30px; height: 30px; display: flex; align-items: center; justify-content: center;
    border: none; background: transparent; border-radius: ${tk.radiusSm}px;
    color: ${tk.textMuted}; cursor: pointer; transition: all 0.15s;
    &:hover { background: ${tk.inkSubtle}; color: ${tk.ink}; }
    &:disabled { opacity: 0.3; cursor: not-allowed; }
  `,
  actBtnDanger: css`&:hover { background: rgba(220, 38, 38, 0.06); color: ${tk.danger}; }`,
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
  const { t } = useTranslation()
  const { styles } = useStyles()
  const { data: skills = [], isLoading } = useSkills()
  const deleteSkill = useDeleteSkill()
  const canWrite = useCanWrite()

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
      message.error(t('skills.downloadFail'))
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
          {skill.url ? t('skills.uploaded') : t('skills.noFile')}
        </span>
      }
      description={skill.description || skill.descriptionEn || t('skills.noDescription')}
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
            title={t('common.download')}
            disabled={!skill.url}
            onClick={() => handleDownload(skill)}
          >
            <ArrowDownIcon size={14} />
          </button>
          {canWrite && (
            <>
              <button type="button" className={styles.actBtn} title={t('common.edit')} onClick={() => { showEdit(skill); }}>
                <PencilSimpleIcon size={14} />
              </button>
              <Popconfirm
                title={t('skills.deleteConfirmTitle')}
                description={t('skills.deleteConfirm', { name: skill.name })}
                okText={t('common.delete')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={() => { deleteSkill.mutate(skill.name); }}
              >
                <button type="button" className={`${styles.actBtn} ${styles.actBtnDanger}`} title={t('common.delete')}>
                  <TrashIcon size={14} />
                </button>
              </Popconfirm>
            </>
          )}
        </div>
      }
    />
  )

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>{t('skills.pageTitle')}</div>
          <div className={styles.pageSub}>{t('skills.pageSub')}</div>
        </div>
        {canWrite && (
          <PrimaryButton
            icon={<PlusIcon size={16} weight="bold" />}
            onClick={() => { setEditingSkill(null); setFormOpen(true) }}
          >
            {t('skills.create')}
          </PrimaryButton>
        )}
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder={t('skills.searchPlaceholder')}
            onSearch={setKeywords}
            realtime
          />
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}><Spin size="medium" /></div>
      ) : filteredSkills.length === 0 ? (
        <div className={styles.emptyState}>
          <div style={{ marginBottom: 20 }}><StarIcon size={48} weight="thin" color={tk.textMuted} /></div>
          <div className={styles.emptyTitle}>{keywords ? t('skills.empty.noMatch') : t('skills.empty.none')}</div>
          <div className={styles.emptyDesc}>{keywords ? t('skills.empty.noMatchHint') : t('skills.empty.noneHint')}</div>
        </div>
      ) : (
        <>
          {expertSkills.length > 0 && (
            <div className={styles.section}>
              <div className={styles.sectionHeader}>
                <div className={styles.sectionTitle}>
                  <MedalIcon size={18} weight="duotone" />
                  {t('skills.expertSection')}
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
                  <UsersThreeIcon size={18} weight="duotone" />
                  {t('skills.communitySection')}
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
