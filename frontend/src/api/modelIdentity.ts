import type { MultiRAGModel } from './multirag'

/** Model references are opaque strings; preserve signed-64-bit IDs exactly. */
export function modelReference(model: MultiRAGModel): string | undefined {
  if (model.reference !== undefined) {
    if (typeof model.reference !== 'string' || !/^[1-9][0-9]{0,18}$/.test(model.reference) || BigInt(model.reference) > 9223372036854775807n || model.fullId !== model.reference) return undefined
    return model.reference
  }
  return model.fullId === `${model.name}@${model.factory}` ? model.fullId : undefined
}
