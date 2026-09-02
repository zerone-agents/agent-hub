// frontend/src/features/agent-chat/useAttachments.ts
import { useCallback, useEffect, useRef, useState } from 'react'
import { agentChatApi, type AttachmentDesc } from '@/api/agent-chat'

/** 客户端提前拦截的限额（与服务端 10 / 20MB / 50MB 同值）。 */
export const ATTACHMENT_LIMITS = {
  maxFiles: 10,
  maxFileBytes: 20 * 1024 * 1024,
  maxTotalBytes: 50 * 1024 * 1024,
} as const

export interface AttachmentItem {
  /** 本地条目 id（非 runtime 描述符 id） */
  id: string
  file: File
  /** 图片本地预览 blob URL（非图片为 null）；移除/清空/卸载时 revoke */
  previewUrl: string | null
  status: 'local' | 'uploading' | 'uploaded'
  descriptor?: AttachmentDesc
}

export function useAttachments() {
  const [items, setItems] = useState<AttachmentItem[]>([])
  const [uploading, setUploading] = useState(false)
  // ref 是同步真值来源：add() 需要立读当前队列做累计校验
  const itemsRef = useRef<AttachmentItem[]>([])
  const seq = useRef(0)

  const commit = useCallback((next: AttachmentItem[]) => {
    itemsRef.current = next
    setItems(next)
  }, [])

  // 卸载时 revoke 所有本地预览 URL
  useEffect(() => {
    return () => {
      for (const it of itemsRef.current) {
        if (it.previewUrl) URL.revokeObjectURL(it.previewUrl)
      }
    }
  }, [])

  /** 批量校验并加入本地队列；返回错误文案（null = 成功）。 */
  const add = useCallback(
    (files: File[]): string | null => {
      if (files.length === 0) return null
      const current = itemsRef.current
      const merged = [...current.map((i) => i.file), ...files]
      if (merged.length > ATTACHMENT_LIMITS.maxFiles) {
        return `附件最多 ${ATTACHMENT_LIMITS.maxFiles} 个`
      }
      let total = 0
      for (const f of merged) {
        if (f.size > ATTACHMENT_LIMITS.maxFileBytes) {
          return `「${f.name}」超过单文件 20MB 上限`
        }
        total += f.size
      }
      if (total > ATTACHMENT_LIMITS.maxTotalBytes) {
        return '附件总大小超过 50MB 上限'
      }
      commit([
        ...current,
        ...files.map((f) => ({
          id: `att-${Date.now()}-${++seq.current}`,
          file: f,
          previewUrl: f.type.startsWith('image/') ? URL.createObjectURL(f) : null,
          status: 'local' as const,
        })),
      ])
      return null
    },
    [commit]
  )

  const remove = useCallback(
    (id: string) => {
      const target = itemsRef.current.find((i) => i.id === id)
      if (target?.previewUrl) URL.revokeObjectURL(target.previewUrl)
      commit(itemsRef.current.filter((i) => i.id !== id))
    },
    [commit]
  )

  /** 发送成功（SSE 建立后）清空全部条目。 */
  const clearAll = useCallback(() => {
    for (const it of itemsRef.current) {
      if (it.previewUrl) URL.revokeObjectURL(it.previewUrl)
    }
    commit([])
  }, [commit])

  /** attachment_missing 重试路径：丢弃已上传描述符，恢复为本地文件。 */
  const invalidate = useCallback(() => {
    commit(itemsRef.current.map((i) => ({ ...i, status: 'local' as const, descriptor: undefined })))
  }, [commit])

  /**
   * 上传全部条目；已全部 uploaded 时直接返回缓存描述符（重试不重复上传）。
   * 返回描述符数组（与 items 顺序一致——runtime 按 multipart 流序返回）。
   * 失败时条目回退 local 并 rethrow。
   */
  const upload = useCallback(
    async (agentName: string, sessionId: string): Promise<AttachmentDesc[]> => {
      const current = itemsRef.current
      if (current.length === 0) return []
      // 不变式：status==='uploaded' 必然带 descriptor；用类型谓词收窄（repo lint 同时禁 as 去空与 !）
      const cached = current.filter(
        (i): i is AttachmentItem & { descriptor: AttachmentDesc } =>
          i.status === 'uploaded' && i.descriptor !== undefined
      )
      if (cached.length === current.length) {
        return cached.map((i) => i.descriptor)
      }
      setUploading(true)
      commit(current.map((i) => ({ ...i, status: 'uploading' as const })))
      try {
        const descs = await agentChatApi.uploadFiles(agentName, sessionId, current.map((i) => i.file))
        commit(current.map((i, idx) => ({ ...i, status: 'uploaded' as const, descriptor: descs[idx] })))
        return descs
      } catch (err) {
        commit(current.map((i) => ({ ...i, status: 'local' as const })))
        throw err
      } finally {
        setUploading(false)
      }
    },
    [commit]
  )

  return { items, uploading, add, remove, clearAll, invalidate, upload }
}
