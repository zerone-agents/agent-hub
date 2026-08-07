import { useEffect, useState, lazy, Suspense } from 'react'
import { Modal, Form, Input, Select, Upload, Spin, Button } from 'antd'
import type { UploadProps } from 'antd'
import { X, UploadSimple } from '@phosphor-icons/react'
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
      setUploadError('仅支持 .zip 格式文件')
      return Upload.LIST_IGNORE
    }
    // Filename charset must match the backend identifier rule — fail fast
    // here so the user can rename, instead of bouncing off the API.
    if (!isValidIdentifier(file.name)) {
      setUploadError('文件名只能包含字母、数字、点、下划线和横线')
      return Upload.LIST_IGNORE
    }
    if (file.size > 50 * 1024 * 1024) {
      setUploadError('文件大小不能超过 50MB')
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
        setSkillMdError(err instanceof Error ? err.message : '解析 SKILL.md 失败')
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
        setUploadError('请选择要上传的 .zip 文件')
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
        <div className={styles.title}>{editingSkill ? '编辑技能' : '新建技能'}</div>
        <button type="button" className={styles.closeBtn} onClick={onClose}>
          <X size={18} />
        </button>
      </div>

      <div className={styles.content}>
        <Form form={form} layout="vertical" className={styles.formCol} requiredMark={false}>
          <div className={styles.section}>基本信息</div>
          <Form.Item label="技能标识" name="name" rules={identifierFormRules('技能标识')}>
            <Input placeholder="e.g. webapp-testing" disabled={!!editingSkill} />
          </Form.Item>
          <Form.Item label="技能类型" name="type" rules={[{ required: true }]}>
            <Select disabled={!!editingSkill} options={[
              { label: '专家 (Expert)', value: 'expert' },
              { label: '社区 (Community)', value: 'community' }
            ]} />
          </Form.Item>

          <div className={styles.section} style={{ marginTop: 20 }}>显示设置</div>
          <Form.Item label="展示名称" name="title">
            <Input placeholder="技能名称" />
          </Form.Item>
          <Form.Item label="Display Name (EN)" name="titleEn">
            <Input placeholder="Skill name" />
          </Form.Item>
          <Form.Item label="功能描述" name="description">
            <Input.TextArea placeholder="描述此技能的功能用途" rows={2} />
          </Form.Item>
          <Form.Item label="Description (EN)" name="descriptionEn">
            <Input.TextArea placeholder="Describe this skill" rows={2} />
          </Form.Item>

          <div className={styles.section} style={{ marginTop: 20 }}>上传文件</div>
          <Upload beforeUpload={beforeUpload} showUploadList={false} accept=".zip" maxCount={1}>
            <button type="button" className={styles.uploadBtn}>
              <UploadSimple size={16} />
              {selectedFile ? selectedFile.name : '选择 .zip 文件'}
            </button>
          </Upload>
          {uploadError && <div className={styles.uploadError}>{uploadError}</div>}
          {!uploadError && (
            <div className={styles.uploadHint}>
              {editingSkill
                ? '留空则保留原文件，选择新文件将替换'
                : 'ZIP 包内须包含 SKILL.md（位于根目录或子目录均可），最大 50MB'}
            </div>
          )}
        </Form>

        <div className={styles.previewCol}>
          <div className={styles.previewHead}>SKILL.md 预览</div>
          <Suspense fallback={<Spin size="small" />}>
            <SkillMdPreview
              loading={skillMdLoading}
              entries={skillMdEntries}
              error={skillMdError}
              placeholder='选择 zip 文件后预览'
            />
          </Suspense>
        </div>
      </div>

      <div className={styles.foot}>
        <Button onClick={onClose}>取消</Button>
        <PrimaryButton onClick={handleSubmit} loading={submitting}>
          {editingSkill ? '更新' : '创建'}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
