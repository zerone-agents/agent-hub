import type { NativeMemoryMessage } from '@/api/memory'

export function newestFirst(a: NativeMemoryMessage, b: NativeMemoryMessage) {
  if (a.createdAt && b.createdAt && a.createdAt !== b.createdAt)
    return b.createdAt.localeCompare(a.createdAt)
  if (a.id.length !== b.id.length) return b.id.length - a.id.length
  return b.id.localeCompare(a.id)
}

export interface MemoryTurnGroup {
  id: string
  source?: NativeMemoryMessage
  messages: NativeMemoryMessage[]
  children: NativeMemoryMessage[]
}

export function memoryTurnGroups(
  messages: NativeMemoryMessage[]
): MemoryTurnGroup[] {
  const groups = new Map<string, MemoryTurnGroup>()
  for (const message of messages) {
    const id = message.type === 'raw' ? message.id : message.sourceId
    const group = groups.get(id) ?? { id, messages: [], children: [] }
    group.messages.push(message)
    if (message.type === 'raw') group.source = message
    else group.children.push(message)
    groups.set(id, group)
  }
  return [...groups.values()].sort((a, b) =>
    newestFirst(a.source ?? a.messages[0], b.source ?? b.messages[0])
  )
}

export function memoryPreview(content: string) {
  return content.replace(/^User Input:\s*/, '')
}

export function formatMemoryCapacity(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}
