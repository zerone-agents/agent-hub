import { KnowledgeDocument, KnowledgeFolder, DocumentType } from '../types';
import { getAuthHeader } from './auth';

/**
 * 知识库 API 层 —— 对接 agent-hub 知识库能力（/api/v1/admin/knowledge，multirag RAG）。
 * 本地开发时由 server.ts 代理到 AGENT_HUB_URL（默认 http://localhost:8081 mock）。
 *
 * 新建知识库一律使用默认配置项（与桌面端 KnowledgeForm 默认值一致）：
 *   权限: 仅自己(me) · 解析方法: 通用(naive) · Embedding: BAAI/bge-large-zh-v1.5
 *   解析布局: DeepDOC · 分块 token: 512 · 分隔符: \n!?。；！？
 *   自动关键词/自动问题: 0 · Excel 转 HTML: 关
 */

const BASE = '/api/v1/admin/knowledge';

/** 新建知识库默认项（图2/图3 配置固化，用户只需填名称+描述） */
export const DATASET_DEFAULTS = {
  permission: 'me',
  parser_id: 'naive',
  embd_id: 'BAAI/bge-large-zh-v1.5',
  parser_config: {
    layout_recognize: 'DeepDOC',
    chunk_token_num: 512,
    delimiter: '\n!?。；！？',
    auto_keywords: 0,
    auto_questions: 0,
    html4excel: false,
  },
} as const;

interface Envelope<T> {
  success: boolean;
  data: T;
  error?: string;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // 真实后端 /api/v1/admin/** 需 JWT（read 组 member+ / write 组 maintainer+；guest 恒 403）。
  // 注意 multipart 上传不能手动设 content-type，这里只补 Authorization。
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: { ...getAuthHeader(), ...(init?.headers ?? {}) },
  });
  if (!res.ok) {
    throw new Error(`知识库接口错误 ${res.status}`);
  }
  const env = (await res.json()) as Envelope<T>;
  if (!env.success) {
    throw new Error(env.error || '知识库接口返回失败');
  }
  return env.data;
}

// ── 后端原始类型（agent-hub 归一化后的字段）──────────────────────
interface RemoteDataset {
  id: string;
  name: string;
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
  run?: string;
  create_time?: number;
  update_time?: number;
}

// ── 字段映射 ────────────────────────────────────────────────────
function formatTime(ts?: number): string {
  if (!ts) return '刚刚';
  const d = new Date(ts);
  const diff = Date.now() - d.getTime();
  if (diff < 60_000) return '刚刚';
  if (diff < 3600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
  if (diff < 86400_000) return `${Math.floor(diff / 3600_000)} 小时前`;
  if (diff < 86400_000 * 30) return `${Math.floor(diff / 86400_000)} 天前`;
  return d.toISOString().split('T')[0];
}

function formatSize(bytes?: number): string {
  if (!bytes || bytes <= 0) return '—';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function suffixToType(name: string, suffix?: string): DocumentType {
  const ext = (suffix || name.split('.').pop() || '').toLowerCase();
  if (ext === 'xlsx' || ext === 'xls' || ext === 'csv') return 'xlsx';
  if (ext === 'html' || ext === 'htm') return 'html';
  if (ext === 'pdf') return 'pdf';
  if (ext === 'md' || ext === 'markdown' || ext === 'txt') return 'md';
  return 'docx';
}

export function datasetToFolder(ds: RemoteDataset): KnowledgeFolder {
  return {
    id: ds.id,
    name: ds.name,
    description: ds.description || '',
    docCount: ds.doc_num ?? 0,
    chunkCount: ds.chunk_num ?? 0,
    parseMethod: ds.parser_id || 'naive',
    category: ds.permission === 'me' ? 'mine' : 'team',
    createdAt: formatTime(ds.create_time),
    updatedAt: formatTime(ds.update_time),
  };
}

export function remoteDocToKnowledgeDoc(doc: RemoteDocument, datasetId: string, datasetName: string): KnowledgeDocument {
  return {
    id: doc.id,
    name: doc.name,
    type: suffixToType(doc.name, doc.suffix || doc.type),
    category: 'mine',
    folderId: datasetId,
    folderName: datasetName,
    size: formatSize(doc.size),
    updatedAt: formatTime(doc.update_time || doc.create_time),
    content: '',
    summary: doc.run === 'DONE' ? `已解析 · ${doc.chunk_num ?? 0} 分块` : '解析中…',
    tags: doc.run === 'DONE' ? ['已切块'] : ['解析中'],
  };
}

// ── 知识库（文件夹 = dataset）────────────────────────────────────
export async function listDatasets(): Promise<KnowledgeFolder[]> {
  const data = await request<{ total: number; datasets: RemoteDataset[] }>('/datasets?page_size=100');
  return (data.datasets || []).map(datasetToFolder);
}

/** 新建知识库：只传名称+描述，解析配置全部走默认项 */
export async function createDataset(name: string, description: string): Promise<KnowledgeFolder> {
  const data = await request<RemoteDataset>('/datasets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name,
      description,
      permission: DATASET_DEFAULTS.permission,
      parser_id: DATASET_DEFAULTS.parser_id,
      embd_id: DATASET_DEFAULTS.embd_id,
      parser_config: DATASET_DEFAULTS.parser_config,
    }),
  });
  return datasetToFolder(data);
}

export async function updateDataset(id: string, patch: { name?: string; description?: string }): Promise<void> {
  await request<unknown>(`/datasets/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
}

export async function deleteDatasets(ids: string[]): Promise<void> {
  await request<unknown>('/datasets', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids }),
  });
}

// ── 文档（文件 = document）──────────────────────────────────────
// 注意：multirag 的 page_size 上限是 100（超了返回 422），大库必须分页拉全。
export async function listDocuments(datasetId: string, datasetName: string): Promise<KnowledgeDocument[]> {
  type DocsPage = { total: number; docs?: RemoteDocument[]; documents?: RemoteDocument[]; items?: RemoteDocument[] };
  const pick = (data: DocsPage) => data.docs || data.documents || data.items || [];

  const all: RemoteDocument[] = [];
  let page = 1;
  // 最多 10 页（1000 个文件）防御死循环
  for (let i = 0; i < 10; i++) {
    const data = await request<DocsPage>(
      `/datasets/${encodeURIComponent(datasetId)}/documents?page=${page}&page_size=100`
    );
    const docs = pick(data);
    all.push(...docs);
    if (docs.length < 100 || all.length >= (data.total ?? 0)) break;
    page++;
  }
  return all.map((d) => remoteDocToKnowledgeDoc(d, datasetId, datasetName));
}

/** 上传文件：multipart/form-data，字段名 files（与 agent-hub 桌面端一致），可多文件 */
export async function uploadDocuments(datasetId: string, datasetName: string, files: File[]): Promise<KnowledgeDocument[]> {
  const form = new FormData();
  files.forEach((f) => form.append('files', f));
  const res = await fetch(`${BASE}/datasets/${encodeURIComponent(datasetId)}/documents`, {
    method: 'POST',
    body: form,
  });
  if (!res.ok) throw new Error(`上传失败 ${res.status}`);
  const env = (await res.json()) as Envelope<RemoteDocument[]>;
  if (!env.success) throw new Error(env.error || '上传失败');
  return (env.data || []).map((d) => remoteDocToKnowledgeDoc(d, datasetId, datasetName));
}

export async function renameDocument(datasetId: string, docId: string, name: string): Promise<void> {
  await request<unknown>(`/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(docId)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
}

export async function deleteDocuments(datasetId: string, ids: string[]): Promise<void> {
  await request<unknown>(`/datasets/${encodeURIComponent(datasetId)}/documents`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids }),
  });
}
