import { KnowledgeDocument, KnowledgeFolder, DocumentType } from "../types";
import { getAuthHeader, getStoredAuth } from "./auth";

/** Mobile knowledge operations use the existing authorized Hub gateway. */

const BASE = "/api/v1/admin/knowledge";

/** H5 新建知识库的预设配置；不能把不可用模型当作创建成功。 */
export const DATASET_DEFAULTS = {
  permission: "me",
  parser_id: "naive",
  embd_id: "BAAI/bge-large-zh-v1.5",
  parser_config: {
    layout_recognize: "DeepDOC",
    chunk_token_num: 512,
    delimiter: "\n!?。；！？",
    auto_keywords: 0,
    auto_questions: 0,
    html4excel: false,
  },
} as const;

interface Envelope<T> {
  success: boolean;
  data: T;
  error?: string;
  code?: string;
}

export class KnowledgeConfigurationPendingError extends Error {
  constructor(
    public folder: KnowledgeFolder,
    message: string,
  ) {
    super(message);
  }
}

export interface UploadResume {
  ids: string[];
  stage: "parse" | "refresh";
}
export class KnowledgeUploadPendingError extends Error {
  constructor(
    public resume: UploadResume,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getStoredAuth()?.token;
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: { ...getAuthHeader(), ...(init?.headers ?? {}) },
  });
  if (token !== getStoredAuth()?.token)
    throw new Error("登录身份已变化，请重新加载知识库");
  let env: Envelope<T>;
  try {
    env = (await res.json()) as Envelope<T>;
  } catch {
    throw new Error(`知识库接口错误 ${res.status}`);
  }
  if (token !== getStoredAuth()?.token)
    throw new Error("登录身份已变化，请重新加载知识库");
  if (!res.ok || !env.success) {
    if (env.code === "knowledge_configuration_pending") {
      const dataset = (env.data as { dataset?: RemoteDataset })?.dataset;
      if (dataset?.id)
        throw new KnowledgeConfigurationPendingError(
          datasetToFolder(dataset),
          env.error || "知识库已创建，配置待保存；重试将使用已有知识库",
        );
    }
    throw new Error(env.error || `知识库接口错误 ${res.status}`);
  }
  return env.data;
}

// ── 后端原始类型（agent-hub 归一化后的字段）──────────────────────
interface RemoteDataset {
  id: string;
  name: string;
  display_name?: string;
  description?: string;
  permission?: string;
  parser_id?: string;
  embd_id?: string;
  doc_num?: number;
  chunk_num?: number;
  create_time?: number;
  update_time?: number;
}

interface RemoteDocument {
  id: string;
  name: string;
  suffix?: string;
  type?: string;
  size?: number;
  chunk_num?: number;
  run?: string | number;
  progress?: number;
  progress_msg?: string;
  create_time?: number;
  update_time?: number;
}

// ── 字段映射 ────────────────────────────────────────────────────
function formatTime(ts?: number): string {
  if (!ts) return "刚刚";
  const d = new Date(ts);
  const diff = Date.now() - d.getTime();
  if (diff < 60_000) return "刚刚";
  if (diff < 3600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
  if (diff < 86400_000) return `${Math.floor(diff / 3600_000)} 小时前`;
  if (diff < 86400_000 * 30) return `${Math.floor(diff / 86400_000)} 天前`;
  return d.toISOString().split("T")[0];
}

function formatSize(bytes?: number): string {
  if (!bytes || bytes <= 0) return "—";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function suffixToType(name: string, suffix?: string): DocumentType {
  const ext = (suffix || name.split(".").pop() || "").toLowerCase();
  if (ext === "xlsx" || ext === "xls" || ext === "csv") return "xlsx";
  if (ext === "html" || ext === "htm") return "html";
  if (ext === "pdf") return "pdf";
  if (ext === "md" || ext === "markdown" || ext === "txt") return "md";
  return "docx";
}

export function datasetToFolder(ds: RemoteDataset): KnowledgeFolder {
  return {
    id: ds.id,
    name: ds.display_name || ds.name,
    description: ds.description || "",
    docCount: ds.doc_num ?? 0,
    chunkCount: ds.chunk_num ?? 0,
    parseMethod: ds.parser_id || "naive",
    category: ds.permission === "me" ? "mine" : "team",
    createdAt: formatTime(ds.create_time),
    updatedAt: formatTime(ds.update_time),
  };
}

export function remoteDocToKnowledgeDoc(
  doc: RemoteDocument,
  datasetId: string,
  datasetName: string,
): KnowledgeDocument {
  const run = String(doc.run ?? "");
  const statuses: Record<string, string> = {
    "0": "未开始解析",
    UNSTART: "未开始解析",
    "1": "解析中",
    RUNNING: "解析中",
    "2": "已取消解析",
    CANCEL: "已取消解析",
    "3": "已完成解析",
    DONE: "已完成解析",
    "4": "解析失败",
    FAIL: "解析失败",
  };
  const status =
    (doc.progress ?? 0) < 0 ? "解析失败" : statuses[run] || "状态未知";
  return {
    id: doc.id,
    name: doc.name,
    type: suffixToType(doc.name, doc.suffix || doc.type),
    category: "mine",
    folderId: datasetId,
    folderName: datasetName,
    size: formatSize(doc.size),
    updatedAt: formatTime(doc.update_time || doc.create_time),
    content: "",
    summary: `${status} · ${doc.chunk_num ?? 0} 分块${status === "已完成解析" && !doc.chunk_num ? " · 未提取到可检索内容，请重新解析或在桌面排查" : ""}${status === "解析失败" && doc.progress_msg ? ` · ${doc.progress_msg}` : ""}`,
    tags: [status],
    run:
      (doc.progress ?? 0) < 0
        ? "4"
        : { UNSTART: "0", RUNNING: "1", CANCEL: "2", DONE: "3", FAIL: "4" }[
            run
          ] || run,
  };
}

async function collectPages<T extends { id: string }>(
  load: (page: number) => Promise<{ total?: number; items: T[] }>,
): Promise<T[]> {
  const all: T[] = [];
  const seen = new Set<string>();
  for (let page = 1; page <= 1000; page++) {
    const data = await load(page);
    const fresh = data.items.filter((item) => !seen.has(item.id));
    if (data.items.length && !fresh.length)
      throw new Error("知识库分页未前进，请重试");
    fresh.forEach((item) => seen.add(item.id));
    all.push(...fresh);
    if (
      data.items.length < 100 &&
      typeof data.total === "number" &&
      all.length < data.total
    )
      throw new Error("知识库分页结果不完整，请重试");
    if (
      data.items.length < 100 ||
      (typeof data.total === "number" && all.length >= data.total)
    )
      return all;
  }
  throw new Error("知识库列表过大，请在桌面端分页查看");
}

// ── 知识库（文件夹 = dataset）────────────────────────────────────
export async function listDatasets(): Promise<KnowledgeFolder[]> {
  const all = await collectPages<RemoteDataset>(async (page) => {
    const data = await request<{ total?: number; datasets: RemoteDataset[] }>(
      `/datasets?page=${page}&page_size=100`,
    );
    return { total: data.total, items: data.datasets || [] };
  });
  return all.map(datasetToFolder);
}

/** 保留已创建ID；配置补写重试不得重复POST。 */
export async function createDataset(
  name: string,
  description: string,
  existingId?: string,
  configuration?: DatasetCreationConfiguration,
): Promise<KnowledgeFolder> {
  const data = await request<RemoteDataset>(
    existingId ? `/datasets/${encodeURIComponent(existingId)}` : "/datasets",
    {
      method: existingId ? "PUT" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name,
        description,
        ...DATASET_DEFAULTS,
        ...(configuration
          ? {
              embd_id: configuration.embeddingModel,
              parser_id: configuration.parseMethod,
            }
          : {}),
      }),
    },
  );
  // The durable ID must survive a failed configuration readback.
  try {
    const saved = await getDataset(data.id);
    if (
      configuration &&
      (saved.embd_id !== configuration.embeddingModel ||
        saved.parser_id !== configuration.parseMethod)
    )
      throw new Error("模型或解析方法尚未保存，请重试保存配置");
    return datasetToFolder(saved);
  } catch (err) {
    throw new KnowledgeConfigurationPendingError(
      datasetToFolder(data),
      `知识库已创建，配置读回失败：${err instanceof Error ? err.message : "请重试"}`,
    );
  }
}

export async function updateDataset(
  id: string,
  patch: { name?: string; description?: string },
): Promise<KnowledgeFolder> {
  const data = await request<RemoteDataset>(
    `/datasets/${encodeURIComponent(id)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    },
  );
  return datasetToFolder(await getDataset(data.id));
}

export async function deleteDatasets(ids: string[]): Promise<void> {
  await request<unknown>("/datasets", {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  });
}

// ── 文档（文件 = document）──────────────────────────────────────
// 知识库与文档均按固定版本的100条页大小拉取，禁止静默截断。
export async function listDocuments(
  datasetId: string,
  datasetName: string,
): Promise<KnowledgeDocument[]> {
  type DocsPage = {
    total?: number;
    docs?: RemoteDocument[];
    documents?: RemoteDocument[];
    items?: RemoteDocument[];
  };
  const all = await collectPages<RemoteDocument>(async (page) => {
    const data = await request<DocsPage>(
      `/datasets/${encodeURIComponent(datasetId)}/documents?page=${page}&page_size=100`,
    );
    return {
      total: data.total,
      items: data.docs || data.documents || data.items || [],
    };
  });
  return all.map((d) => remoteDocToKnowledgeDoc(d, datasetId, datasetName));
}

export async function parseDocuments(
  datasetId: string,
  ids: string[],
): Promise<void> {
  await request(`/datasets/${encodeURIComponent(datasetId)}/documents/parse`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ document_ids: ids }),
  });
}

export async function ingestDocuments(
  ids: string[],
  options: { delete: boolean; apply_kb: boolean },
): Promise<void> {
  const accepted = await request<unknown>("/documents/ingest", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ doc_ids: ids, run: 1, ...options }),
  });
  if (accepted !== true) throw new Error("解析请求未确认受理，请刷新后重试");
}

export async function stopParsingDocuments(
  datasetId: string,
  ids: string[],
): Promise<void> {
  await request(`/datasets/${encodeURIComponent(datasetId)}/documents/parse`, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ document_ids: ids }),
  });
}

/** 上传文件：multipart/form-data，字段名 files（与 agent-hub 桌面端一致），可多文件 */
export async function uploadDocuments(
  datasetId: string,
  datasetName: string,
  files: File[],
): Promise<KnowledgeDocument[]> {
  const form = new FormData();
  files.forEach((f) => form.append("files", f));
  const data = await request<RemoteDocument[]>(
    `/datasets/${encodeURIComponent(datasetId)}/documents`,
    { method: "POST", body: form },
  );
  return (data || []).map((d) =>
    remoteDocToKnowledgeDoc(d, datasetId, datasetName),
  );
}

/** Uploaded IDs survive parse/readback failures, so resuming never reposts files. */
export async function completeUpload(
  datasetId: string,
  datasetName: string,
  files: File[],
  autoParse: boolean,
  onUploaded: (documents: KnowledgeDocument[]) => void,
  refresh: () => Promise<void>,
  resume?: UploadResume,
): Promise<void> {
  const token = getStoredAuth()?.token;
  let ids = resume?.ids;
  if (!ids) {
    const created = await uploadDocuments(datasetId, datasetName, files);
    if (!created.length)
      throw new Error("上传未返回文档，请先刷新确认上传结果");
    ids = created.map((doc) => doc.id);
    onUploaded(created);
  }
  if (token !== getStoredAuth()?.token)
    throw new Error("登录身份已变化，请重新加载知识库");
  if (autoParse && resume?.stage !== "refresh") {
    try {
      await parseDocuments(datasetId, ids);
    } catch (err) {
      throw new KnowledgeUploadPendingError(
        { ids, stage: "parse" },
        `文件已上传，解析请求失败：${err instanceof Error ? err.message : "请重试"}`,
      );
    }
  }
  try {
    await refresh();
  } catch (err) {
    throw new KnowledgeUploadPendingError(
      { ids, stage: "refresh" },
      `文件已上传，状态刷新失败：${err instanceof Error ? err.message : "请重试"}`,
    );
  }
}

export async function renameDocument(
  datasetId: string,
  docId: string,
  name: string,
): Promise<void> {
  await request<unknown>(
    `/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(docId)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    },
  );
}

export async function deleteDocuments(
  datasetId: string,
  ids: string[],
): Promise<void> {
  await request<unknown>(
    `/datasets/${encodeURIComponent(datasetId)}/documents`,
    {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ids }),
    },
  );
}

export interface DatasetCreationConfiguration {
  embeddingModel: string;
  parseMethod: string;
}

export const MOBILE_PARSERS = [
  {
    id: "naive",
    label: "通用文档",
    description: "适合 PDF、Word、文本等常见资料",
  },
  { id: "qa", label: "问答", description: "适合已有问题与答案的资料" },
  { id: "table", label: "表格", description: "适合按行检索的结构化表格" },
  { id: "manual", label: "手册", description: "适合按章节组织的操作手册" },
] as const;

export async function getDataset(id: string): Promise<RemoteDataset> {
  return request(`/datasets/${encodeURIComponent(id)}`);
}

export async function listEmbeddingModels(
  signal?: AbortSignal,
): Promise<{ fullId: string; name: string; factory: string }[]> {
  const data = await request<
    {
      fullId: string;
      name: string;
      factory: string;
      status: string;
      type: string;
    }[]
  >("/multirag/models?type=embedding", { signal });
  return data.filter(
    (model) =>
      model.status === "1" &&
      model.type.toLowerCase() === "embedding" &&
      model.fullId,
  );
}

export interface MobileChunk {
  id: string;
  content: string;
  documentId: string;
  documentName: string;
  imageId?: string;
  available: boolean;
  positions: unknown[];
  similarity?: number;
}

function normalizeMobileChunk(raw: Record<string, unknown>): MobileChunk {
  const positions = raw.positions ?? raw.position_int;
  return {
    id: String(raw.id ?? raw.chunk_id ?? ""),
    content: String(raw.content ?? raw.content_with_weight ?? raw.text ?? ""),
    documentId: String(raw.document_id ?? raw.doc_id ?? ""),
    documentName: String(
      raw.document_name ?? raw.docnm_kwd ?? raw.document_keyword ?? "",
    ),
    imageId:
      typeof (raw.image_id ?? raw.img_id) === "string"
        ? String(raw.image_id ?? raw.img_id)
        : undefined,
    available:
      (raw.available ?? raw.available_int ?? 1) !== false &&
      Number(raw.available ?? raw.available_int ?? 1) !== 0,
    positions: Array.isArray(positions) ? positions : [],
    similarity:
      typeof raw.similarity === "number"
        ? raw.similarity
        : typeof raw.score === "number"
          ? raw.score
          : undefined,
  };
}

export function chunkPageNumbers(positions: unknown[]): number[] {
  // MultiRAG's PDF positions are [one-based page, left, right, top, bottom].
  return [
    ...new Set(
      positions
        .filter(Array.isArray)
        .map((position) => position[0])
        .filter(
          (page): page is number =>
            typeof page === "number" && Number.isInteger(page) && page > 0,
        ),
    ),
  ];
}

export async function listDocumentChunks(
  datasetId: string,
  documentId: string,
  page = 1,
  signal?: AbortSignal,
): Promise<{ total: number; chunks: MobileChunk[] }> {
  const data = await request<{
    total: number;
    chunks: Record<string, unknown>[];
  }>(
    `/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/chunks?page=${page}&page_size=20`,
    { signal },
  );
  if (!Array.isArray(data.chunks) || !Number.isFinite(data.total))
    throw new Error("切片结果不完整，请重试");
  return { total: data.total, chunks: data.chunks.map(normalizeMobileChunk) };
}

export async function searchKnowledge(
  datasetId: string,
  question: string,
  page = 1,
  documentId?: string,
  signal?: AbortSignal,
): Promise<{ total: number; chunks: MobileChunk[] }> {
  if (!question.trim()) throw new Error("请输入要核对的问题");
  const data = await request<{
    total: number;
    chunks: Record<string, unknown>[];
  }>("/retrieval", {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      question: question.trim(),
      dataset_ids: [datasetId],
      ...(documentId ? { document_ids: [documentId] } : {}),
      page,
      page_size: 10,
      top_k: 1024,
      similarity_threshold: 0.2,
      vector_similarity_weight: 0.3,
      highlight: false,
      use_kg: false,
    }),
  });
  if (!Array.isArray(data.chunks) || !Number.isFinite(data.total))
    throw new Error("检索结果不完整，请重试");
  return { total: data.total, chunks: data.chunks.map(normalizeMobileChunk) };
}

async function authorizedBlob(
  path: string,
  signal?: AbortSignal,
): Promise<Blob> {
  const token = getStoredAuth()?.token;
  const res = await fetch(`${BASE}${path}`, {
    headers: getAuthHeader(),
    signal,
    cache: "no-store",
  });
  if (!res.ok) {
    let message = `内容读取失败 ${res.status}`;
    try {
      message = (await res.json()).error || message;
    } catch {
      /* preserve status */
    }
    throw new Error(message);
  }
  const blob = await res.blob();
  if (token !== getStoredAuth()?.token)
    throw new Error("登录身份已变化，请重新加载知识库");
  if (!blob.size) throw new Error("内容为空，请重试");
  return blob;
}

export async function downloadOriginal(
  datasetId: string,
  documentId: string,
  signal?: AbortSignal,
): Promise<Blob> {
  return authorizedBlob(
    `/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/download`,
    signal,
  );
}

export async function loadChunkImage(
  datasetId: string,
  imageId: string,
  signal?: AbortSignal,
): Promise<Blob> {
  const blob = await authorizedBlob(
    `/datasets/${encodeURIComponent(datasetId)}/images/${encodeURIComponent(imageId)}`,
    signal,
  );
  if (!/^image\/(png|jpeg|gif|webp|bmp)$/.test(blob.type))
    throw new Error("图片格式不支持预览");
  return blob;
}
