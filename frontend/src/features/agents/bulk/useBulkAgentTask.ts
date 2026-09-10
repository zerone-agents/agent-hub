import { useCallback, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { agentApi } from '@/api/agents'
import { parseApiError } from '@/api/client'
import type { Agent } from '@/api/agents'
import type { BulkOperation, ClassifiedItem } from './classifyBulkOperation'

export type BulkItemStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'skipped' | 'blocked'

export interface BulkTaskItem {
  name: string
  title: string
  status: BulkItemStatus
  reason?: string
}

/** 执行状态与展示状态正交（spec §6）：done 的「已查看」取决于完成时刻的 presentation。 */
export type TaskPhase = 'idle' | 'running' | 'done'
export type TaskPresentation = 'modal-open' | 'collapsed' | 'dismissed'

export interface BulkTaskSummary {
  total: number
  succeeded: number
  failed: number
  skipped: number
  blocked: number
}

export interface StartBulkTaskOptions {
  operation: BulkOperation
  /** 完整分类结果（含跳过/受限项，进入进度模型） */
  items: ClassifiedItem[]
  /** 批次全部结束时回调成功名单（页面用于清理选中项） */
  onFinished?: (succeededNames: string[]) => void
}

/** 并发上限（spec §6）：deployer 重操作限 2，轻操作 5。 */
const CONCURRENCY: Record<BulkOperation, number> = {
  deploy: 2,
  redeploy: 2,
  stop: 5,
  delete: 5,
}

function runOperation(operation: BulkOperation, name: string): Promise<unknown> {
  switch (operation) {
    case 'deploy': return agentApi.deploy(name)
    case 'redeploy': return agentApi.deploy(name, true)
    case 'stop': return agentApi.stopDeployment(name)
    case 'delete': return agentApi.delete(name)
  }
}

function titleOf(agent: Agent): string {
  return agent.config.title?.zh ?? agent.config.title?.en ?? agent.name
}

export function useBulkAgentTask() {
  const queryClient = useQueryClient()
  const [phase, setPhase] = useState<TaskPhase>('idle')
  const [presentation, setPresentation] = useState<TaskPresentation>('dismissed')
  const [operation, setOperation] = useState<BulkOperation | null>(null)
  const [items, setItems] = useState<BulkTaskItem[]>([])

  const start = useCallback((opts: StartBulkTaskOptions) => {
    const { operation: op, items: classified, onFinished } = opts
    // 跳过/受限项初始化为终态；可执行项排队
    const taskItems: BulkTaskItem[] = classified.map((it) =>
      it.classification === 'executable'
        ? { name: it.agent.name, title: titleOf(it.agent), status: 'pending' as const }
        : { name: it.agent.name, title: titleOf(it.agent), status: it.classification, reason: it.reason },
    )
    setOperation(op)
    setItems(taskItems)
    setPhase('running')
    setPresentation('modal-open')

    const updateItem = (name: string, patch: Partial<BulkTaskItem>) => {
      setItems((prev) => prev.map((it) => (it.name === name ? { ...it, ...patch } : it)))
    }

    const queue = taskItems.filter((it) => it.status === 'pending').map((it) => it.name)
    const succeeded: string[] = []
    let cursor = 0
    let settled = 0

    const finish = () => {
      setPhase('done')
      void queryClient.invalidateQueries({ queryKey: ['agents'] })
      onFinished?.(succeeded)
    }

    if (queue.length === 0) {
      finish() // 仅跳过/受限批次（hook 直接输入场景）
      return
    }

    const worker = async (): Promise<void> => {
      while (cursor < queue.length) {
        const name = queue[cursor]!
        cursor++ // JS 单线程，检查与自增之间无 await，无竞态
        updateItem(name, { status: 'running' })
        try {
          await runOperation(op, name)
          succeeded.push(name)
          updateItem(name, { status: 'succeeded' })
        } catch (err) {
          updateItem(name, { status: 'failed', reason: parseApiError(err) })
        } finally {
          settled++
          if (settled === queue.length) finish()
        }
      }
    }

    void Promise.all(Array.from({ length: Math.min(CONCURRENCY[op], queue.length) }, () => worker()))
  }, [queryClient])

  /** 执行中收起（任务继续后台执行） */
  const collapse = useCallback(() => {
    setPresentation((p) => (p === 'modal-open' ? 'collapsed' : p))
  }, [])

  /** 气泡点击重新展开 */
  const reopen = useCallback(() => {
    setPresentation((p) => (p === 'collapsed' ? 'modal-open' : p))
  }, [])

  /** 仅 done 后可调用：清除任务回到 idle */
  const close = useCallback(() => {
    setPhase('idle')
    setPresentation('dismissed')
    setOperation(null)
    setItems([])
  }, [])

  const summary: BulkTaskSummary = {
    total: items.length,
    succeeded: items.filter((i) => i.status === 'succeeded').length,
    failed: items.filter((i) => i.status === 'failed').length,
    skipped: items.filter((i) => i.status === 'skipped').length,
    blocked: items.filter((i) => i.status === 'blocked').length,
  }

  return { phase, presentation, operation, items, summary, start, collapse, reopen, close }
}
