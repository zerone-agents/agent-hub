import type { KnowledgeObject } from "@/api/knowledgeManagement";

export function showKnowledgeValue(value: unknown): string {
  return value === undefined || value === null || value === ""
    ? "—"
    : typeof value === "string" ||
        typeof value === "number" ||
        typeof value === "boolean"
      ? String(value)
      : JSON.stringify(value);
}

export function knowledgeRows(value: unknown): KnowledgeObject[] {
  return Array.isArray(value)
    ? value.filter(
        (item: unknown): item is KnowledgeObject =>
          Boolean(item) && typeof item === "object" && !Array.isArray(item),
      )
    : [];
}

export function knowledgeObject(value: unknown): KnowledgeObject | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as KnowledgeObject)
    : undefined;
}

export function graphNodeId(node: KnowledgeObject): string {
  return showKnowledgeValue(node.id ?? node.name ?? node.entity_name);
}

export function graphEndpoint(
  edge: KnowledgeObject,
  side: "source" | "target",
): string {
  const value = edge[side] ?? edge[side === "source" ? "src_id" : "tgt_id"];
  return knowledgeObject(value)
    ? graphNodeId(knowledgeObject(value) ?? {})
    : showKnowledgeValue(value);
}

export type IndexState =
  | "empty"
  | "queued"
  | "running"
  | "done"
  | "failed"
  | "cancelRequested"
  | "endedUnconfirmed"
  | "unknown";

export function cancellationMarkerCount(message: unknown): number {
  return typeof message === "string"
    ? message.split("[cancel_requested]").length - 1
    : 0;
}

export function getIndexState(
  task: KnowledgeObject | undefined,
  cancellationBaseline?: { taskId: unknown; markerCount: number },
): IndexState {
  if (!task) return "unknown";
  if (Object.keys(task).length === 0) return "empty";
  const progress = task.progress;
  if (
    typeof progress !== "number" ||
    !Number.isFinite(progress) ||
    typeof task.id !== "string" ||
    !task.id.trim()
  )
    return "unknown";
  if (progress >= 1) return "done";
  if (progress < 0) {
    const markers = cancellationMarkerCount(task.progress_msg);
    if (progress !== -1 || markers === 0) return "failed";
    if (cancellationBaseline?.taskId !== task.id)
      return "endedUnconfirmed";
    return markers > cancellationBaseline.markerCount
      ? "cancelRequested"
      : "failed";
  }
  return progress === 0 ? "queued" : "running";
}

export function logDocumentId(log: KnowledgeObject): string | undefined {
  const document = log.document_id;
  return typeof document === "string" &&
    document &&
    document !== "graph_raptor_x" &&
    document !== "dataflow_x"
    ? document
    : undefined;
}

export function ingestionLogResource({
  page,
  logType,
  keywords,
  status,
  from,
  to,
  desc,
}: {
  page: number;
  logType: "file" | "dataset";
  keywords: string;
  status: string[];
  from?: string;
  to?: string;
  desc: boolean;
}): string {
  // Repeated unbracketed parameters are required by the MultiRAG REST model.
  const query = new URLSearchParams({
    page: String(page),
    page_size: "15",
    log_type: logType,
    orderby: "create_time",
    desc: String(desc),
  });
  if (keywords.trim()) query.set("keywords", keywords.trim());
  status.forEach((value) => {
    query.append("operation_status", value);
  });
  if (from) query.set("create_date_from", from);
  if (to) query.set("create_date_to", to);
  return `ingestions?${query.toString()}`;
}
