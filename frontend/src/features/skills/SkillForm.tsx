import { useEffect, useState, lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import { Modal, Form, Input, Select, Upload, Spin, Button } from 'antd'
import type { UploadProps } from 'antd'
import { XIcon, UploadSimpleIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Skill } from '@/api/skills'
import { useCreateSkill, useUpdateSkill } from '@/queries/useSkills'
import { useSkillMd } from '@/queries/useSkillMd'
import { identifierFormRules, isValidIdentifier } from '@/utils/identifier'
import { tokens as t } from '@/styles/tokens'
import { parseSkillMd, type SkillMdEntry } from './parseSkillMd'

// Lazy: pulls in @lobehub/ui Markdown — heavy, only needed when the form modal opens
const SkillMdPreview = lazy(() => import('./SkillMdPreview'))

const useStyles = createStyles(({ css }) => ({
  head: css`
    display: flex; justify-content: space-between; align-items: center;
    padding: 18px 24px; border-bottom: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  title: css`
    font-size: 18px; font-weight: 600; color: var(--text); letter-spacing: -0.02em;
  `,
  closeBtn: css`
    width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
    border: none; background: var(--ink-subtle); border-radius: 4px;
    color: var(--text-tertiary); cursor: pointer; transition: all 0.15s;
    &:hover { background: var(--ink-light); color: var(--text); }
  `,
  body: css`
    padding: 0;
  `,
  content: css`
    display: flex; max-height: 60vh; overflow: hidden;
  `,
  formCol: css`
    width: 440px; overflow-y: auto;
    padding: 20px 24px 8px;
    border-right: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  previewCol: css`
    flex: 1; overflow: hidden;
    display: flex; flex-direction: column;
    padding: 20px 24px;
  `,
  previewHead: css`
    font-size: 11px; font-weight: 600; color: var(--text-muted);
    text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 14px;
  `,
  section: css`
    font-size: 11px; font-weight: 600; color: var(--text-muted);
    text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 14px;
  `,
  uploadBtn: css`
    display: inline-flex; align-items: center; gap: 8px;
    padding: 8px 16px; background: var(--paper); color: var(--text-secondary);
    border: 1px dashed color-mix(in srgb, var(--foreground) 20%, transparent); border-radius: 4px;
    font-family: ${t.fontSans}; font-size: 13px; font-weight: 500;
    cursor: pointer; transition: all 0.15s;
    &:hover { border-color: var(--ink); color: var(--ink); }
  `,
  uploadError: css`
    margin-top: 8px; font-size: 12px; color: ${t.danger};
  `,
  uploadHint: css`
    margin-top: 8px; font-size: 12px; color: ${t.textTertiary};
  `,
  foot: css`
    display: flex; justify-content: flex-end; gap: 10px;
    padding: 14px 24px; border-top: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `
}))

interface SkillFormProps {
  open: boolean
  editingSkill: Skill | null
  onClose: () => void
}

interface FormValues {
  name: string
  type: string
  title: string
  titleEn: string
  description: string
  descriptionEn: string
}

export default function SkillForm({ open, editingSkill, onClose }: SkillFormProps) {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const createSkill = useCreateSkill()
  const updateSkill = useUpdateSkill()
  const submitting = createSkill.isPending || updateSkill.isPending

  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [uploadError, setUploadError] = useState('')
  const [skillMdEntries, setSkillMdEntries] = useState<SkillMdEntry[]>([])
  const [skillMdLoading, setSkillMdLoading] = useState(false)
  const [skillMdError, setSkillMdError] = useState('')

  // 编辑模式：Modal 打开时从后端拉取已存技能的 SKILL.md
  const { data: remoteMd, isLoading: remoteLoading, error: remoteError } = useSkillMd(
    open && editingSkill ? editingSkill.name : null
  )

  useEffect(() => {
    if (open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- reset upload-related local state on modal open; coupled to the antd form.setFieldsValue below
      setSelectedFile(null)
      setUploadError('')
      setSkillMdEntries([])
      setSkillMdError('')
      if (editingSkill) {
        form.setFieldsValue({
          name: editingSkill.name,
          type: editingSkill.type || 'expert',
          title: editingSkill.title || '',
          titleEn: editingSkill.titleEn || '',
          description: editingSkill.description || '',
          descriptionEn: editingSkill.descriptionEn || ''
        })
      } else {
        form.resetFields()
        form.setFieldsValue({ type: 'expert' })
      }
    }
  }, [open, editingSkill, form])

  // 编辑模式：远程 SKILL.md 数据到达后更新本地状态。
  // useSkillMd 现在返回 SkillMdEntry[]，直接喂给预览组件。
  // Bundle zip 会在预览区显示 tab 切换。
  useEffect(() => {
    if (editingSkill && !selectedFile) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- mirror remote SKILL.md query state into local state so the preview component can consume it
      setSkillMdLoading(remoteLoading)
      if (remoteError) {
        setSkillMdError(remoteError.message)
        setSkillMdEntries([])
      } else if (remoteMd !== undefined) {
        setSkillMdEntries(remoteMd)
        setSkillMdError('')
      }
    }
  }, [editingSkill, selectedFile, remoteMd, remoteLoading, remoteError])

  const beforeUpload: UploadProps['beforeUpload'] = (file) => {
    if (!file.name.endsWith('.zip')) {
      // 校验失败必须同时清掉已持有的文件，否则按钮仍显示旧文件名、提交会带上旧文件
      setSelectedFile(null)
      setUploadError(t('skills.form.zipOnly'))
      return Upload.LIST_IGNORE
    }
    // Filename charset must match the backend identifier rule — fail fast
    // here so the user can rename, instead of bouncing off the API.
    if (!isValidIdentifier(file.name)) {
      setSelectedFile(null)
      setUploadError(t('skills.form.filenameCharset'))
      return Upload.LIST_IGNORE
    }
    if (file.size > 50 * 1024 * 1024) {
      setSelectedFile(null)
      setUploadError(t('skills.form.tooLarge'))
      return Upload.LIST_IGNORE
    }
    setUploadError('')
    setSelectedFile(file)
    // 选择新文件后用 JSZip 即时解析预览
    setSkillMdError('')
    setSkillMdLoading(true)
    parseSkillMd(file)
      .then((entries) => {
        setSkillMdEntries(entries)
        setSkillMdLoading(false)
      })
      .catch((err: unknown) => {
        setSkillMdEntries([])
        setSkillMdError(err instanceof Error ? err.message : t('skills.form.parseFail'))
        setSkillMdLoading(false)
      })
    return false // prevent auto-upload
  }

  const handleSubmit = async () => {
    const values = await form.validateFields()

    if (editingSkill) {
      await updateSkill.mutateAsync({
        name: editingSkill.name,
        data: {
          title: values.title,
          titleEn: values.titleEn,
          description: values.description,
          descriptionEn: values.descriptionEn,
          file: selectedFile ?? undefined
        }
      })
    } else {
      if (!selectedFile) {
        setUploadError(t('skills.form.zipRequired'))
        return
      }
      const formData = new FormData()
      formData.append('name', values.name)
      formData.append('type', values.type)
      formData.append('title', values.title)
      formData.append('titleEn', values.titleEn)
      formData.append('description', values.description)
      formData.append('descriptionEn', values.descriptionEn)
      formData.append('file', selectedFile)
      await createSkill.mutateAsync(formData)
    }
    onClose()
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable={false}
      width={1000}
      styles={{ body: { padding: 0 } }}
      title={null}
      destroyOnHidden
    >
      <div className={styles.head}>
        <div className={styles.title}>{editingSkill ? t('skills.form.editTitle') : t('skills.create')}</div>
        <button type="button" className={styles.closeBtn} onClick={onClose}>
          <XIcon size={18} />
        </button>
      </div>

      <div className={styles.content}>
        <Form form={form} layout="vertical" className={styles.formCol} requiredMark={false}>
          <div className={styles.section}>{t('skills.form.basicSection')}</div>
          <Form.Item label={t('skills.form.nameKey')} name="name" rules={identifierFormRules(t('skills.form.nameKey'))}>
            <Input placeholder="e.g. webapp-testing" disabled={!!editingSkill} />
          </Form.Item>
          <Form.Item label={t('skills.form.typeLabel')} name="type" rules={[{ required: true }]}>
            <Select disabled={!!editingSkill} options={[
              { label: t('skills.form.typeExpert'), value: 'expert' },
              { label: t('skills.form.typeCommunity'), value: 'community' }
            ]} />
          </Form.Item>

          <div className={styles.section} style={{ marginTop: 20 }}>{t('skills.form.displaySection')}</div>
          <Form.Item label={t('skills.form.titleLabel')} name="title">
            <Input placeholder={t('skills.form.titlePlaceholder')} />
          </Form.Item>
          <Form.Item label="Display Name (EN)" name="titleEn">
            <Input placeholder="Skill name" />
          </Form.Item>
          <Form.Item label={t('skills.form.descLabel')} name="description">
            <Input.TextArea placeholder={t('skills.form.descPlaceholder')} rows={2} />
          </Form.Item>
          <Form.Item label="Description (EN)" name="descriptionEn">
            <Input.TextArea placeholder="Describe this skill" rows={2} />
          </Form.Item>

          <div className={styles.section} style={{ marginTop: 20 }}>{t('skills.form.uploadSection')}</div>
          <Upload beforeUpload={beforeUpload} showUploadList={false} accept=".zip" maxCount={1}>
            <button type="button" className={styles.uploadBtn}>
              <UploadSimpleIcon size={16} />
              {selectedFile ? selectedFile.name : t('skills.form.selectFile')}
            </button>
          </Upload>
          {uploadError && <div className={styles.uploadError}>{uploadError}</div>}
          {!uploadError && (
            <div className={styles.uploadHint}>
              {editingSkill
                ? t('skills.form.keepFileHint')
                : t('skills.form.zipHint')}
            </div>
          )}
        </Form>

        <div className={styles.previewCol}>
          <div className={styles.previewHead}>{t('skills.form.previewTitle')}</div>
          <Suspense fallback={<Spin size="small" />}>
            <SkillMdPreview
              loading={skillMdLoading}
              entries={skillMdEntries}
              error={skillMdError}
              placeholder={t('skills.form.previewPlaceholder')}
            />
          </Suspense>
        </div>
      </div>

      <div className={styles.foot}>
        <Button onClick={onClose}>{t('common.cancel')}</Button>
        <PrimaryButton onClick={handleSubmit} loading={submitting}>
          {editingSkill ? t('scenes.update') : t('scenes.createSubmit')}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
