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
  // uploadingRef 与 uploading 同步镜像：队列变更入口据此冻结（见 Global Constraints）
  const uploadingRef = useRef(false)
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

  /** 批量校验并加入本地队列；返回错误文案（null = 成功）。
   *  uploading 冻结：拒绝新增（见 Global Constraints 队列冻结契约）。 */
  const add = useCallback(
    (files: File[]): string | null => {
      if (files.length === 0) return null
      if (uploadingRef.current) return '上传进行中，请稍候再添加'
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
      if (uploadingRef.current) return // uploading 冻结：静默拒绝
      const target = itemsRef.current.find((i) => i.id === id)
      if (target?.previewUrl) URL.revokeObjectURL(target.previewUrl)
      commit(itemsRef.current.filter((i) => i.id !== id))
    },
    [commit]
  )

  /** 发送成功（SSE 建立后）清空全部条目。uploading 冻结：静默拒绝。 */
  const clearAll = useCallback(() => {
    if (uploadingRef.current) return
    for (const it of itemsRef.current) {
      if (it.previewUrl) URL.revokeObjectURL(it.previewUrl)
    }
    commit([])
  }, [commit])

  /** attachment_missing 重试路径：丢弃已上传描述符，恢复为本地文件。uploading 冻结：静默拒绝。 */
  const invalidate = useCallback(() => {
    if (uploadingRef.current) return
    commit(itemsRef.current.map((i) => ({ ...i, status: 'local' as const, descriptor: undefined })))
  }, [commit])

  /**
   * 上传全部条目；已全部 uploaded 时直接返回缓存描述符（重试不重复上传）。
   * 返回描述符数组（与 items 顺序一致——runtime 按 multipart 流序返回）。
   * 失败时条目回退 local 并 rethrow。
   */
  const upload = useCallback(
    async (agentName: string, sessionId: string): Promise<AttachmentDesc[]> => {
      if (uploadingRef.current) throw new Error('上传进行中，请稍候')
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
      uploadingRef.current = true
      setUploading(true)
      commit(current.map((i) => ({ ...i, status: 'uploading' as const })))
      try {
        const descs = await agentChatApi.uploadFiles(agentName, sessionId, current.map((i) => i.file))
        // 描述符数量必须与提交数一一对应（runtime 按 multipart 流序返回）。
        // 数量异常若不拦截，多余条目会静默丢附件。
        if (descs.length !== current.length) {
          throw new Error(`上传响应异常：期望 ${current.length} 个附件描述，得到 ${descs.length} 个`)
        }
        commit(current.map((i, idx) => ({ ...i, status: 'uploaded' as const, descriptor: descs[idx] })))
        return descs
      } catch (err) {
        commit(current.map((i) => ({ ...i, status: 'local' as const })))
        throw err
      } finally {
        uploadingRef.current = false
        setUploading(false)
      }
    },
    [commit]
  )

  return { items, uploading, add, remove, clearAll, invalidate, upload }
}
