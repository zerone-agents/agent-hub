import { useEffect, useState } from 'react'
import { Modal, Form, Input, Upload, Button } from 'antd'
import type { UploadProps } from 'antd'
import { XIcon, UploadSimpleIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Tool } from '@/api/tools'
import { useCreateCustomTool, useUploadToolFile, useUpdateTool } from '@/queries/useTools'
import { identifierFormRules } from '@/utils/identifier'
import { tokens as t } from '@/styles/tokens'

export type ToolFormMode = 'create' | 'edit' | 'upload'

// 与 deployer 契约一致：扩展名大小写敏感
const ALLOWED_EXTENSIONS = ['.ts', '.mts', '.js', '.mjs']
const MAX_FILE_SIZE = 5 * 1024 * 1024

const useStyles = createStyles(({ css }) => ({
  modalHead: css`
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 18px 24px;
    border-bottom: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  modalTitle: css`
    font-size: 18px;
    font-weight: 600;
    color: var(--text);
    letter-spacing: -0.02em;
  `,
  modalClose: css`
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: var(--ink-subtle);
    border-radius: 4px;
    color: var(--text-tertiary);
    cursor: pointer;
    transition: all 0.15s;
    &:hover {
      background: var(--ink-light);
      color: var(--text);
    }
  `,
  modalBody: css`
    padding: 20px 24px 8px;
    max-height: 60vh;
    overflow-y: auto;
  `,
  sectionTitle: css`
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 14px;
  `,
  uploadBtn: css`
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    background: var(--paper);
    color: var(--text-secondary);
    border: 1px dashed color-mix(in srgb, var(--foreground) 20%, transparent);
    border-radius: 4px;
    font-family: ${t.fontSans};
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s;
    &:hover {
      border-color: var(--ink);
      color: var(--ink);
    }
  `,
  uploadError: css`
    margin-top: 8px;
    font-size: 12px;
    color: ${t.danger};
  `,
  uploadHint: css`
    margin-top: 8px;
    font-size: 12px;
    color: ${t.textTertiary};
  `,
  hintBlock: css`
    margin-top: 16px;
    padding: 10px 12px;
    background: var(--ink-subtle);
    border-radius: 4px;
    font-size: 12px;
    color: var(--text-tertiary);
    line-height: 1.8;
  `,
  modalFoot: css`
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    padding: 14px 24px;
    border-top: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `
}))

export interface ToolFormProps {
  open: boolean
  mode: ToolFormMode
  editingTool: Tool | null
  onClose: () => void
}

interface FormValues {
  name: string
  title: string
  description: string
}

export default function ToolForm({ open, mode, editingTool, onClose }: ToolFormProps) {
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const createCustomTool = useCreateCustomTool()
  const updateTool = useUpdateTool()
  const uploadToolFile = useUploadToolFile()
  const submitting = createCustomTool.isPending || updateTool.isPending || uploadToolFile.isPending

  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [uploadError, setUploadError] = useState('')

  useEffect(() => {
    if (open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- reset upload-related local state on modal open; coupled to the antd form.setFieldsValue below
      setSelectedFile(null)
      setUploadError('')
      if (mode === 'create' || !editingTool) {
        form.resetFields()
      } else {
        form.setFieldsValue({
          name: editingTool.name,
          title: editingTool.title || '',
          description: editingTool.description || ''
        })
      }
    }
  }, [open, mode, editingTool, form])

  const beforeUpload: UploadProps['beforeUpload'] = (file) => {
    const ext = file.name.substring(file.name.lastIndexOf('.'))
    if (!ALLOWED_EXTENSIONS.includes(ext)) {
      setUploadError('仅支持 .ts / .mts / .js / .mjs 文件')
      return Upload.LIST_IGNORE
    }
    if (file.size === 0) {
      setUploadError('文件不能为空')
      return Upload.LIST_IGNORE
    }
    if (file.size > MAX_FILE_SIZE) {
      setUploadError('文件大小不能超过 5MB')
      return Upload.LIST_IGNORE
    }
    setUploadError('')
    setSelectedFile(file)
    return false // prevent auto-upload
  }

  const handleSubmit = async () => {
    const file = selectedFile
    if (mode !== 'edit' && !file) {
      setUploadError('请选择工具文件')
      return
    }

    if (mode === 'upload') {
      if (!editingTool || !file) return
      await uploadToolFile.mutateAsync({ name: editingTool.name, file })
      onClose()
      return
    }

    const values = await form.validateFields()
    if (mode === 'create') {
      if (!file) return
      await createCustomTool.mutateAsync({
        name: values.name,
        title: values.title,
        description: values.description,
        file
      })
    } else {
      if (!editingTool) return
      // 先更新元数据，再（可选）上传替换文件
      await updateTool.mutateAsync({
        name: editingTool.name,
        data: { title: values.title, description: values.description }
      })
      if (file) {
        await uploadToolFile.mutateAsync({ name: editingTool.name, file })
      }
    }
    onClose()
  }

  const modalTitle = mode === 'create' ? '上传自定义工具' : mode === 'edit' ? '编辑工具' : '补传/替换工具文件'
  const submitText = mode === 'create' ? '上传' : mode === 'edit' ? '更新' : '补传'
  const showMetadata = mode !== 'upload'

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable={false}
      width={560}
      styles={{ body: { padding: 0 } }}
      title={null}
      destroyOnHidden
    >
      <div className={styles.modalHead}>
        <div className={styles.modalTitle}>{modalTitle}</div>
        <button type="button" className={styles.modalClose} onClick={onClose}>
          <XIcon size={18} />
        </button>
      </div>

      <Form form={form} layout="vertical" className={styles.modalBody} requiredMark={false}>
        {showMetadata && (
          <>
            <div className={styles.sectionTitle}>基本信息</div>
            <Form.Item
              label="工具标识"
              name="name"
              rules={identifierFormRules('工具标识')}
            >
              <Input placeholder="e.g. SayHello" disabled={mode !== 'create'} />
            </Form.Item>

            <div className={styles.sectionTitle} style={{ marginTop: 20 }}>显示设置</div>
            <Form.Item label="中文名称" name="title">
              <Input placeholder="中文名称" />
            </Form.Item>
            <Form.Item label="功能描述" name="description">
              <Input.TextArea placeholder="描述此工具的功能用途" rows={3} />
            </Form.Item>
          </>
        )}

        <div className={styles.sectionTitle} style={{ marginTop: showMetadata ? 20 : 0 }}>
          {mode === 'edit' ? '替换文件（可选）' : '工具文件'}
        </div>
        <Upload beforeUpload={beforeUpload} showUploadList={false} accept=".ts,.mts,.js,.mjs" maxCount={1}>
          <button type="button" className={styles.uploadBtn}>
            <UploadSimpleIcon size={16} />
            {selectedFile ? selectedFile.name : '选择 .ts / .mts / .js / .mjs 文件'}
          </button>
        </Upload>
        {uploadError && <div className={styles.uploadError}>{uploadError}</div>}
        {mode === 'edit' && !uploadError && (
          <div className={styles.uploadHint}>留空则保留原文件，选择新文件将替换</div>
        )}
        {mode === 'upload' && !uploadError && (
          <div className={styles.uploadHint}>支持 .ts / .mts / .js / .mjs 文件，最大 5MB</div>
        )}
        {mode === 'create' && (
          <div className={styles.hintBlock}>
            <div>· 工具标识必须与文件内默认导出的 name 一致，部署时由 Runtime 最终校验</div>
            <div>· 仅支持 Node.js 内置模块、@zerone-agent/agent-runtime/tools 与 zod，不安装 npm 依赖</div>
            <div>· 工具将在 Agent Runtime 进程中执行并拥有完整 Node.js 权限，仅上传可信代码</div>
          </div>
        )}
      </Form>

      <div className={styles.modalFoot}>
        <Button onClick={onClose}>取消</Button>
        <PrimaryButton onClick={handleSubmit} loading={submitting}>
          {submitText}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
