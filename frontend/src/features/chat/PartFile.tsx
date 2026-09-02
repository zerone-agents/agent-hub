import { useEffect, useState } from 'react'
import { PaperclipIcon, DownloadIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'
import { authFetchBlob } from '@/api/agent-chat'
import { formatBytes } from '@/utils/format'
import type { ContentPart } from './parts'

/** 附件内容 URL 构造器：交互页用 session 代理，管理页用 admin files 代理。 */
export type AttachmentUrlBuilder = (path: string) => string

const useStyles = createStyles(({ css }) => ({
  card: css`
    display: inline-flex; align-items: center; gap: 8px;
    padding: 8px 12px; border: 1px solid ${t.inkLighter};
    border-radius: 8px; background: ${t.surface};
    max-width: 320px; font-size: 12px; color: ${t.text};
  `,
  meta: css`display: flex; flex-direction: column; min-width: 0;`,
  name: css`
    max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  `,
  size: css`color: ${t.textMuted}; font-size: 11px;`,
  action: css`
    border: none; background: transparent; cursor: pointer; color: ${t.textSecondary};
    display: inline-flex; align-items: center; padding: 4px;
    &:hover { color: ${t.ink}; }
    &:disabled { opacity: 0.5; cursor: default; }
  `,
  image: css`
    max-width: min(360px, 100%); max-height: 240px; border-radius: 8px;
    display: block;
  `,
  imageLoading: css`
    width: 160px; height: 120px; border-radius: 8px;
    background: ${t.inkSubtle};
    animation: imgPulse 1.2s infinite ease-in-out both;
    @keyframes imgPulse { 0%, 80%, 100% { opacity: 0.5; } 40% { opacity: 1; } }
  `,
  unavailable: css`
    display: inline-flex; align-items: center; gap: 6px;
    padding: 6px 10px; border: 1px dashed ${t.inkLighter};
    border-radius: 8px; font-size: 12px; color: ${t.textMuted};
    max-width: 320px;
  `,
}))

interface PartFileProps {
  part: ContentPart
  buildAttachmentUrl?: AttachmentUrlBuilder
}

export default function PartFile({ part, buildAttachmentUrl }: PartFileProps) {
  const { styles } = useStyles()
  const name = typeof part.name === 'string' && part.name ? part.name : (typeof part.path === 'string' && part.path ? part.path : '文件')
  const mime = typeof part.mime === 'string' ? part.mime : ''
  const size = typeof part.size === 'number' ? part.size : undefined
  const path = typeof part.path === 'string' ? part.path : ''
  const isImage = mime.startsWith('image/')

  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)
  const [downloading, setDownloading] = useState(false)

  // 图片：鉴权 fetch → blob URL（token 不入 URL）；卸载/路径变化 revoke
  useEffect(() => {
    if (!buildAttachmentUrl || !isImage || !path) return
    let revoked = false
    let localUrl: string | null = null
    authFetchBlob(buildAttachmentUrl(path))
      .then((blob) => {
        if (revoked) return
        localUrl = URL.createObjectURL(blob)
        setObjectUrl(localUrl)
      })
      .catch(() => {
        if (!revoked) setFailed(true)
      })
    return () => {
      revoked = true
      if (localUrl) URL.revokeObjectURL(localUrl)
      setObjectUrl(null)
    }
  }, [buildAttachmentUrl, isImage, path])

  // 下载不走 <a href> 直链（浏览器导航不带 Authorization 会 401）：
  // 鉴权 fetch → blob → 程序化触发 <a download> → 用后 revoke
  const download = async () => {
    if (!buildAttachmentUrl || !path || downloading) return
    setDownloading(true)
    try {
      const blob = await authFetchBlob(buildAttachmentUrl(path))
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = name
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch {
      setFailed(true)
    } finally {
      setDownloading(false)
    }
  }

  if (failed) {
    return (
      <div className={styles.unavailable}>
        <PaperclipIcon size={12} />
        <span>
          {name}
          {size !== undefined ? ` · ${formatBytes(size)}` : ''} · 临时文件已不可用
        </span>
      </div>
    )
  }

  if (isImage && buildAttachmentUrl) {
    if (objectUrl) {
      return <img className={styles.image} src={objectUrl} alt={name} title={name} loading="lazy" />
    }
    return <div className={styles.imageLoading} aria-label={name} />
  }

  return (
    <div className={styles.card}>
      <PaperclipIcon size={14} />
      <div className={styles.meta}>
        <span className={styles.name} title={name}>{name}</span>
        {size !== undefined && <span className={styles.size}>{formatBytes(size)}</span>}
      </div>
      {buildAttachmentUrl ? (
        <button
          type="button"
          className={styles.action}
          onClick={() => { void download(); }}
          disabled={downloading}
          title="下载"
          aria-label={`下载 ${name}`}
        >
          <DownloadIcon size={14} />
        </button>
      ) : (
        <span className={styles.size}>仅元数据</span>
      )}
    </div>
  )
}
