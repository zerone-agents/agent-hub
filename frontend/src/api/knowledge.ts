import apiClient from "./client";
import type { AxiosResponse } from "axios";
import { ownedRequestConfig, type RequestOwner } from "./requestOwnership";
import i18next from "@/i18n";

/**
 * Knowledge base API client + field anti-corruption layer.
 *
 * The browser only ever talks to control-panel's own admin gateway at
 * `/api/v1/admin/knowledge/*`. It never reaches multirag directly and no
 * multirag base URL / API key appears here. The gateway already remaps most
 * multirag transport field names to stable domain names; this adapter mirrors
 * that mapping defensively (stable name first, multirag-native name as
 * fallback, then a default) so pages only ever consume stable typed fields:
 * `doc_num`, `chunk_num`, `parser_id`, `embd_id`, `content`,
 * `important_keywords`, `questions`.
 */

const BASE = "/api/v1/admin/knowledge";
const DOCUMENT_UPLOAD_TIMEOUT_MS = 60 * 60 * 1000;

type RawObject = Record<string, unknown>;

interface Envelope<T> {
  success: boolean;
  data: T;
  message?: string;
  error?: string;
}

// ---------------------------------------------------------------------------
// Stable domain types (what pages consume)
// ---------------------------------------------------------------------------

export interface KnowledgeDataset {
  id: string;
  name: string;
  display_name: string;
  collection_name: string;
  description: string;
  permission: string;
  doc_num: number;
  chunk_num: number;
  parser_id: string;
  embd_id: string;
  parser_config: Record<string, unknown>;
  create_time?: number;
  update_time?: number;
  create_date?: string;
  update_date?: string;
  [key: string]: unknown;
}

export interface KnowledgeDocument {
  id: string;
  name: string;
  chunk_num: number;
  token_num: number;
  parser_id: string;
  run: string;
  progress: number;
  progress_msg: string;
  status: string;
  enabled: boolean;
  size: number;
  type: string;
  meta_fields: Record<string, unknown>[];
  parser_config: Record<string, unknown>;
  created_by?: string;
  nickname?: string;
  process_begin_at?: number;
  process_duration?: number;
  source_type?: string;
  thumbnail?: string;
  suffix?: string;
  create_time?: number;
  update_time?: number;
  create_date?: string;
  update_date?: string;
  [key: string]: unknown;
}

export interface KnowledgeChunk {
  id: string;
  content: string;
  document_id: string;
  important_keywords: string[];
  questions: string[];
  available: boolean;
  positions: unknown[];
  doc_type?: string;
  tag_kwd: string[];
  tag_feas: Record<string, unknown>;
  image_id?: string;
  [key: string]: unknown;
}

export interface ReferenceMetadataConfig {
  include?: boolean;
  fields?: string[] | null;
}

export interface MetadataFilter {
  method: 'manual' | 'auto' | 'semi_auto';
  logic?: 'and' | 'or';
  manual?: { key: string; op: string; value?: unknown }[];
  semi_auto?: (string | { key: string; op?: string })[];
}

export interface RetrievalChunk {
  document_metadata?: Record<string, unknown>;
  reference_metadata?: Record<string, unknown>;
  image_id?: string;
  dataset_id?: string;
  positions?: unknown[];
  id: string;
  content: string;
  document_id: string;
  document_name: string;
  similarity: number;
  vector_similarity: number;
  term_similarity: number;
  highlight?: string;
}

export interface DocAggregation {
  doc_id: string;
  doc_name: string;
  count: number;
}

export interface RetrievalResult {
  reference_metadata?: Record<string, unknown>;
  total: number;
  chunks: RetrievalChunk[];
  doc_aggs: DocAggregation[];
  labels: Record<string, unknown>;
}

export interface DatasetListResult {
  total: number;
  datasets: KnowledgeDataset[];
}

export interface DocumentListResult {
  total: number;
  documents: KnowledgeDocument[];
}

export interface ChunkListResult {
  total: number;
  chunks: KnowledgeChunk[];
  document: KnowledgeDocument | null;
}

// ---------------------------------------------------------------------------
// Request input types
// ---------------------------------------------------------------------------

export type KnowledgeWriteOwner = RequestOwner;

export interface DatasetFormInput {
  name?: string;
  display_name?: string;
  collection_name?: string;
  description?: string;
  permission?: string;
  parser_id?: string;
  embd_id?: string;
  parser_config?: Record<string, unknown>;
}

export interface DatasetListParams {
  id?: string;
  page?: number;
  page_size?: number;
  orderby?: string;
  desc?: boolean;
  keywords?: string;
  parser_id?: string;
}

export interface DocumentListParams {
  ids?: string[];
  types?: string[];
  metadata?: string;
  return_empty_metadata?: boolean;
  id?: string;
  page?: number;
  page_size?: number;
  orderby?: string;
  desc?: boolean;
  keywords?: string;
  suffix?: string[];
  run?: string[];
  create_time_from?: number;
  create_time_to?: number;
  metadata_condition?: string;
}

export type DocumentFilterParams = Pick<DocumentListParams, "keywords" | "run" | "types" | "suffix">;
export interface DocumentFilterResult {
  total: number;
  filter: {
    suffix: Record<string, number>;
    run_status: Record<string, number>;
    metadata: Record<string, Record<string, number>>;
  };
}

export interface DocumentIngestInput {
  doc_ids: string[];
  run: 0 | 1 | 2;
  delete: boolean;
  apply_kb: boolean;
}

export interface ChunkListParams {
  id?: string;
  page?: number;
  page_size?: number;
  keywords?: string;
  available?: boolean;
}

export interface ChunkFormInput {
  image_update_mode?: "append" | "replace";
  content?: string;
  important_keywords?: string[];
  questions?: string[];
  image_base64?: string;
  tag_kwd?: string[];
  tag_feas?: Record<string, unknown>;
}

export interface RetrievalInput {
  size?: number;
  doc_ids?: string[] | null;
  meta_data_filter?: MetadataFilter;
  reference_metadata?: ReferenceMetadataConfig;
  page?: number;
  page_size?: number;
  rerank_id?: string;
  keyword?: boolean;
  use_kg?: boolean;
  cross_languages?: string[];
  metadata_condition?: Record<string, unknown>;
  search_mode?: { type: 'sparse' | 'dense' | 'hybrid' | 'fusion'; weight_dense?: number; weight_sparse?: number; weights?: string };
  question: string;
  dataset_ids: string[];
  document_ids?: string[];
  top_k?: number;
  similarity_threshold?: number;
  vector_similarity_weight?: number;
  highlight?: boolean;
}

// ---------------------------------------------------------------------------
// Primitive coercion helpers (tolerant of the loose map[string]any payloads)
// ---------------------------------------------------------------------------

function pick(raw: RawObject, ...keys: string[]): unknown {
  for (const key of keys) {
    const value = raw[key];
    if (value !== undefined && value !== null) return value;
  }
  return undefined;
}

function str(value: unknown, fallback = ""): string {
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") return String(value);
  if (value === undefined || value === null) return fallback;
  return fallback;
}

function optionalStr(value: unknown): string | undefined {
  if (typeof value === "string") return value === "" ? undefined : value;
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") return String(value);
  return undefined;
}

function num(value: unknown, fallback = 0): number {
  if (typeof value === "number" && !Number.isNaN(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (!Number.isNaN(parsed)) return parsed;
  }
  return fallback;
}

function optionalNum(value: unknown): number | undefined {
  if (typeof value === "number" && !Number.isNaN(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (!Number.isNaN(parsed)) return parsed;
  }
  return undefined;
}

function strArray(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item));
  return [];
}

function unknownArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function isRecord(value: unknown): value is RawObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function recordArray(value: unknown): Record<string, unknown>[] {
  if (Array.isArray(value)) return value.filter(isRecord);
  if (isRecord(value)) return [value];
  return [];
}

function stripDataURLPrefix(value: string): string {
  const marker = "base64,";
  const markerIndex = value.indexOf(marker);
  return markerIndex >= 0 ? value.slice(markerIndex + marker.length) : value;
}

// ---------------------------------------------------------------------------
// Normalizers (anti-corruption boundary — exported for unit testing)
// ---------------------------------------------------------------------------

export function normalizeDataset(raw: RawObject): KnowledgeDataset {
  const name = str(raw.name);
  const displayName = optionalStr(pick(raw, "display_name")) ?? name;
  return {
    ...raw,
    id: str(pick(raw, "id", "dataset_id", "kb_id")),
    name: displayName,
    display_name: displayName,
    collection_name: str(pick(raw, "collection_name")),
    description: str(raw.description),
    permission: str(pick(raw, "permission"), "me"),
    doc_num: num(pick(raw, "doc_num", "document_count")),
    chunk_num: num(pick(raw, "chunk_num", "chunk_count")),
    parser_id: str(pick(raw, "parser_id", "chunk_method"), "naive"),
    embd_id: str(pick(raw, "embd_id", "embedding_model")),
    parser_config: isRecord(raw.parser_config) ? raw.parser_config : {},
    create_time: optionalNum(raw.create_time),
    update_time: optionalNum(raw.update_time),
    create_date: optionalStr(raw.create_date),
    update_date: optionalStr(raw.update_date),
  };
}

export function normalizeDocumentRun(value: unknown): string {
  const state = str(value, "0").trim();
  const states: Record<string, string> = { UNSTART: "0", RUNNING: "1", CANCEL: "2", DONE: "3", FAIL: "4" };
  return states[state.toUpperCase()] ?? state;
}

export function normalizeDocument(raw: RawObject): KnowledgeDocument {
  const enabledRaw = pick(raw, "enabled");
  const statusRaw = str(pick(raw, "status"), "1");
  return {
    ...raw,
    id: str(pick(raw, "id", "doc_id", "document_id")),
    name: str(raw.name),
    chunk_num: num(pick(raw, "chunk_num", "chunk_count")),
    token_num: num(pick(raw, "token_num", "token_count")),
    parser_id: str(pick(raw, "parser_id", "chunk_method"), "naive"),
    run: normalizeDocumentRun(pick(raw, "run")),
    progress: num(pick(raw, "progress")),
    progress_msg: str(pick(raw, "progress_msg")),
    status: statusRaw,
    enabled: typeof enabledRaw === "boolean" ? enabledRaw : statusRaw === "1",
    size: num(pick(raw, "size")),
    type: str(pick(raw, "type")),
    meta_fields: Array.isArray(pick(raw, "meta_fields", "metadata_fields"))
      ? recordArray(pick(raw, "meta_fields", "metadata_fields"))
      : Object.entries(isRecord(raw.meta_fields) ? raw.meta_fields : isRecord(raw.metadata_fields) ? raw.metadata_fields : {}).map(([key, value]) => ({ key, value })),
    parser_config: isRecord(raw.parser_config) ? raw.parser_config : {},
    created_by: optionalStr(
      pick(raw, "created_by", "created_id", "creator_id"),
    ),
    nickname: optionalStr(
      pick(raw, "nickname", "created_by_name", "creator_name"),
    ),
    process_begin_at: optionalNum(
      pick(raw, "process_begin_at", "process_begin_time"),
    ),
    process_duration: optionalNum(raw.process_duration),
    source_type: optionalStr(pick(raw, "source_type", "source")),
    thumbnail: optionalStr(raw.thumbnail),
    suffix: optionalStr(raw.suffix),
    create_time: optionalNum(raw.create_time),
    update_time: optionalNum(raw.update_time),
    create_date: optionalStr(raw.create_date),
    update_date: optionalStr(raw.update_date),
  };
}

export function normalizeChunk(raw: RawObject): KnowledgeChunk {
  const availableRaw = pick(raw, "available", "available_int");
  const available =
    typeof availableRaw === "boolean"
      ? availableRaw
      : num(availableRaw, 1) !== 0;
  return {
    ...raw,
    id: str(pick(raw, "id", "chunk_id")),
    content: str(pick(raw, "content", "content_with_weight", "text")),
    document_id: str(pick(raw, "document_id", "doc_id")),
    important_keywords: strArray(
      pick(raw, "important_keywords", "important_kwd"),
    ),
    questions: strArray(pick(raw, "questions", "question_kwd")),
    available,
    positions: unknownArray(pick(raw, "positions", "position_int")),
    doc_type: optionalStr(pick(raw, "doc_type", "doc_type_kwd")),
    tag_kwd: strArray(pick(raw, "tag_kwd")),
    tag_feas: isRecord(raw.tag_feas) ? raw.tag_feas : {},
    image_id: optionalStr(pick(raw, "image_id", "img_id")),
  };
}

export function normalizeRetrievalChunk(raw: RawObject): RetrievalChunk {
  return {
    document_metadata: isRecord(raw.document_metadata) ? raw.document_metadata : undefined,
    reference_metadata: isRecord(raw.reference_metadata) ? raw.reference_metadata : undefined,
    id: str(pick(raw, "id", "chunk_id")),
    content: str(pick(raw, "content", "content_with_weight", "text")),
    document_id: str(pick(raw, "document_id", "doc_id")),
    document_name: str(
      pick(raw, "document_name", "docnm_kwd", "document_keyword"),
    ),
    similarity: num(pick(raw, "similarity", "score")),
    vector_similarity: num(pick(raw, "vector_similarity")),
    term_similarity: num(pick(raw, "term_similarity")),
    highlight: optionalStr(raw.highlight),
    image_id: optionalStr(pick(raw, "image_id", "img_id")),
    dataset_id: optionalStr(pick(raw, "dataset_id", "kb_id")),
    positions: unknownArray(pick(raw, "positions", "position_int")),
  };
}

export function normalizeRetrievalResult(raw: RawObject): RetrievalResult {
  const chunks = Array.isArray(raw.chunks)
    ? raw.chunks.filter(isRecord).map(normalizeRetrievalChunk)
    : [];
  const docAggs = Array.isArray(raw.doc_aggs)
    ? raw.doc_aggs.filter(isRecord).map((item) => ({
        doc_id: str(pick(item, "doc_id")),
        doc_name: str(pick(item, "doc_name")),
        count: num(pick(item, "count")),
      }))
    : [];
  return {
    total: num(pick(raw, "total")),
    chunks,
    doc_aggs: docAggs,
    labels: isRecord(raw.labels) ? raw.labels : {},
    reference_metadata: isRecord(raw.reference_metadata) ? raw.reference_metadata : undefined,
  };
}

// ---------------------------------------------------------------------------
// Request body builders (exported for unit testing)
// ---------------------------------------------------------------------------

export function toDatasetBody(
  input: DatasetFormInput,
): Record<string, unknown> {
  const body: Record<string, unknown> = {};
  if (input.name !== undefined) body.name = input.name;
  if (input.display_name !== undefined) body.display_name = input.display_name;
  if (input.collection_name !== undefined)
    body.collection_name = input.collection_name;
  if (input.description !== undefined) body.description = input.description;
  if (input.permission !== undefined) body.permission = input.permission;
  // parser_id / embd_id are remapped to chunk_method / embedding_model by the
  // gateway — we send the stable names.
  if (input.parser_id !== undefined) body.parser_id = input.parser_id;
  if (input.embd_id !== undefined) body.embd_id = input.embd_id;
  if (input.parser_config !== undefined)
    body.parser_config = input.parser_config;
  return body;
}

export function toChunkBody(input: ChunkFormInput): Record<string, unknown> {
  const body: Record<string, unknown> = {};
  if (input.content !== undefined) body.content = input.content;
  if (input.important_keywords !== undefined)
    body.important_keywords = input.important_keywords;
  if (input.questions !== undefined) body.questions = input.questions;
  if (input.image_update_mode !== undefined) body.image_update_mode = input.image_update_mode;
  if (input.image_base64 !== undefined)
    body.image_base64 = stripDataURLPrefix(input.image_base64);
  if (input.tag_kwd !== undefined) body.tag_kwd = input.tag_kwd;
  if (input.tag_feas !== undefined) body.tag_feas = input.tag_feas;
  return body;
}

// ---------------------------------------------------------------------------
// Envelope unwrap
// ---------------------------------------------------------------------------

function includesRequestedFields(actual: unknown, expected: unknown): boolean {
  if (Array.isArray(expected)) return Array.isArray(actual) && actual.length === expected.length && expected.every((value, index) => includesRequestedFields(actual[index], value));
  if (isRecord(expected)) return isRecord(actual) && Object.entries(expected).every(([key, value]) => includesRequestedFields(actual[key], value));
  return actual === expected;
}

// Keep legacy local IDs compatible with the gateway's protocol mapping.
// A PUT response may only normalize the reference's shape, not choose a model.
const LOCAL_MODEL_FACTORIES = ['Anthropic', 'OpenAI-API-Compatible', 'MinerU', 'PaddleOCR'];
const LEGACY_LAYOUT_FACTORIES = new Set(['MinerU', 'PaddleOCR', 'Docling', 'OpenDataLoader', 'TCADP Parser']);
const STRICT_LAYOUT_VALUES = new Set(['DeepDOC', 'Plain Text', ...LEGACY_LAYOUT_FACTORIES]);
const KNOWN_MODEL_FACTORIES = new Set([...LOCAL_MODEL_FACTORIES, ...LEGACY_LAYOUT_FACTORIES, 'OpenAI', 'BAAI'].map((factory) => factory.toLowerCase()));

function isKnownFullModelRef(ref: string): boolean {
  const separator = ref.lastIndexOf('@');
  return separator > 0 && KNOWN_MODEL_FACTORIES.has(ref.slice(separator + 1).toLowerCase());
}

function expectedDatasetMutation(input: DatasetFormInput, written: RawObject): RawObject {
  const expected = toDatasetBody(input);
  if (input.embd_id && !isKnownFullModelRef(input.embd_id) && LOCAL_MODEL_FACTORIES.some((factory) => written.embd_id === `${input.embd_id}@${factory}`)) {
    expected.embd_id = written.embd_id;
  }
  if (input.parser_config) {
    const config = { ...input.parser_config };
    const parentChild = isRecord(config.parent_child) ? { ...config.parent_child } : {};
    if (config.enable_children !== undefined) parentChild.use_parent_child = config.enable_children;
    if (config.children_delimiter !== undefined) parentChild.children_delimiter = config.children_delimiter;
    if (Object.keys(parentChild).length) {
      // MultiRAG stores the disabled mode as parent_child: {}, even if the
      // request used use_parent_child: false (and a stale delimiter).
      config.parent_child = parentChild.use_parent_child === false ? {} : parentChild;
      delete config.enable_children;
      delete config.children_delimiter;
    }
    for (const [key, target] of [["raptor", "use_raptor"], ["graphrag", "use_graphrag"]]) {
      const nested = config[key];
      if (isRecord(nested) && nested.enabled !== undefined && nested[target] === undefined) {
        const normalized = { ...nested, [target]: nested.enabled };
        delete normalized.enabled;
        config[key] = normalized;
      }
    }
    if (typeof config.layout_recognize === "string" && config.layout_recognize.length > 0 && !config.layout_recognize.includes("@") && !STRICT_LAYOUT_VALUES.has(config.layout_recognize) && isRecord(written.parser_config) && typeof written.parser_config.layout_recognize === "string" && LEGACY_LAYOUT_FACTORIES.has(written.parser_config.layout_recognize)) {
      config.layout_recognize = written.parser_config.layout_recognize;
    }
    expected.parser_config = config;
  }
  return expected;
}

async function unwrap<T>(
  promise: Promise<AxiosResponse<Envelope<T>>>,
): Promise<T> {
  const res = await promise;
  const body = res.data;
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- axios res.data typed as Envelope<T> but runtime may be undefined for empty/error responses
  if (body && !body.success) {
    throw new Error(body.error ?? body.message ?? i18next.t("apiErrors.requestFailed"));
  }
  return body.data;
}

export function buildQuery(params: Record<string, unknown>): string {
  const query = new URLSearchParams();
  const toPrimitive = (v: unknown): string | undefined => {
    if (typeof v === "string") return v;
    if (typeof v === "number" || typeof v === "boolean" || typeof v === "bigint") return String(v);
    return undefined;
  };
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === "") continue;
    if (Array.isArray(value)) {
      for (const item of value) {
        if (item !== undefined && item !== null && item !== "") {
          const s = toPrimitive(item);
          if (s !== undefined) query.append(key, s);
        }
      }
      continue;
    }
    const s = toPrimitive(value);
    if (s !== undefined) query.set(key, s);
  }
  const qs = query.toString();
  return qs ? `?${qs}` : "";
}

export interface HealthStatus {
  configured: boolean
  connected: boolean
  status: string
  message?: string
}

// ---------------------------------------------------------------------------
// API surface
// ---------------------------------------------------------------------------

export const knowledgeApi = {
  health: async (): Promise<HealthStatus> => {
    interface HealthData {
      configured?: boolean
      connected?: boolean
      status?: string
      message?: string
    }
    interface AxiosErrShape {
      response?: { data?: { data?: HealthData; error?: string }; status?: number }
      message?: string
    }
    try {
      const res = await apiClient.get<{ data?: HealthStatus }>(`${BASE}/health`)
      const data = res.data.data
      return {
        configured: !!data?.configured,
        connected: !!data?.connected,
        status: data?.status ?? 'unknown',
        message: data?.message,
      }
    } catch (err: unknown) {
      // When MULTIRAG is unconfigured the backend returns 503 with success:false,
      // but the data envelope still contains configured/connected/status.
      const axiosErr = err as AxiosErrShape
      const data = axiosErr.response?.data?.data
      return {
        configured: !!data?.configured,
        connected: !!data?.connected,
        status: data?.status ?? 'unavailable',
        message: axiosErr.response?.data?.error ?? data?.message ?? axiosErr.message,
      }
    }
  },

  datasets: {
    list: async (
      params: DatasetListParams = {},
      owner?: KnowledgeWriteOwner,
    ): Promise<DatasetListResult> => {
      owner?.assertCurrent();
      const qs = buildQuery({
        id: params.id,
        page: params.page,
        page_size: params.page_size,
        orderby: params.orderby,
        desc: params.desc,
        keywords: params.keywords,
        parser_id: params.parser_id,
      });
      const data = await unwrap<{ total: number; datasets: RawObject[] }>(
        apiClient.get(`${BASE}/datasets${qs}`, ...(owner ? [ownedRequestConfig(owner)] as const : [] as const)),
      );
      owner?.assertCurrent();
      if (!Number.isInteger(data.total) || data.total < 0 || !Array.isArray(data.datasets)) {
        throw new Error(i18next.t('knowledge.states.listFailed'));
      }
      return {
        total: num(data.total),
        datasets: (data.datasets).map(normalizeDataset),
      };
    },

    get: async (datasetId: string, owner?: KnowledgeWriteOwner): Promise<KnowledgeDataset> => {
      owner?.assertCurrent();
      const data = await unwrap<RawObject>(
        apiClient.get(`${BASE}/datasets/${encodeURIComponent(datasetId)}`, ...(owner ? [ownedRequestConfig(owner)] as const : [] as const)),
      );
      owner?.assertCurrent();
      const dataset = normalizeDataset(data);
      if (dataset.id !== datasetId) throw new Error(i18next.t("knowledge.states.detailFailed"));
      return dataset;
    },

    create: async (input: DatasetFormInput, owner?: KnowledgeWriteOwner): Promise<KnowledgeDataset> => {
      owner?.assertCurrent();
      const data = await unwrap<RawObject>(
        apiClient.post(`${BASE}/datasets`, toDatasetBody(input), ...(owner ? [ownedRequestConfig(owner)] as const : [] as const)),
      );
      owner?.assertCurrent();
      return normalizeDataset(data);
    },

    update: async (
      datasetId: string,
      input: DatasetFormInput,
      owner?: KnowledgeWriteOwner,
    ): Promise<KnowledgeDataset> => {
      owner?.assertCurrent();
      if (!Object.keys(toDatasetBody(input)).length) {
        const current = await unwrap<RawObject>(apiClient.get(`${BASE}/datasets/${encodeURIComponent(datasetId)}`, ...(owner ? [ownedRequestConfig(owner)] as const : [] as const)));
        owner?.assertCurrent();
        const saved = normalizeDataset(current);
        if (saved.id !== datasetId) throw new Error(i18next.t("knowledge.states.detailFailed"));
        return saved;
      }
      const written = await unwrap<RawObject>(
        apiClient.put(`${BASE}/datasets/${encodeURIComponent(datasetId)}`, toDatasetBody(input), ...(owner ? [ownedRequestConfig(owner)] as const : [] as const)),
      );
      owner?.assertCurrent();
      try {
        const data = await unwrap<RawObject>(apiClient.get(`${BASE}/datasets/${encodeURIComponent(datasetId)}`, ...(owner ? [ownedRequestConfig(owner)] as const : [] as const)));
        owner?.assertCurrent();
        const saved = normalizeDataset(data);
        if (saved.id !== datasetId || !includesRequestedFields(saved, expectedDatasetMutation(input, isRecord(written) ? written : {}))) {
          throw new Error("readback mismatch");
        }
        return saved;
      } catch {
        throw new Error(i18next.t("knowledge.settings.readbackFailed"));
      }
    },

    remove: async (datasetIds: string[], owner?: KnowledgeWriteOwner): Promise<unknown> => {
      owner?.assertCurrent();
      const result = await unwrap(apiClient.delete<Envelope<unknown>>(`${BASE}/datasets`, { ...ownedRequestConfig(owner), data: { ids: datasetIds } }));
      owner?.assertCurrent();
      return result;
    },
  },

  documents: {
    list: async (
      datasetId: string,
      params: DocumentListParams = {},
    ): Promise<DocumentListResult> => {
      const qs = buildQuery({
        page: params.page,
        page_size: params.page_size,
        orderby: params.orderby,
        desc: params.desc,
        keywords: params.keywords,
        id: params.id,
        ids: params.ids,
        types: params.types,
        metadata: params.metadata,
        return_empty_metadata: params.return_empty_metadata,
        suffix: params.suffix,
        run: params.run,
        create_time_from: params.create_time_from,
        create_time_to: params.create_time_to,
        metadata_condition: params.metadata_condition,
      });
      const data = await unwrap<{ total: number; documents: RawObject[] }>(
        apiClient.get(
          `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents${qs}`,
        ),
      );
      return {
        total: num(data.total),
        documents: (data.documents).map(normalizeDocument),
      };
    },

    get: async (datasetId: string, documentId: string): Promise<KnowledgeDocument> => {
      const result = await knowledgeApi.documents.list(datasetId, { id: documentId, page: 1, page_size: 1 });
      const document = result.documents.find((item) => item.id === documentId);
      if (!document) throw new Error(i18next.t("knowledge.manage.readbackFailed"));
      return document;
    },

    filters: async (datasetId: string, params: DocumentFilterParams = {}): Promise<DocumentFilterResult> => {
      return unwrap<DocumentFilterResult>(apiClient.get(
        `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/filters${buildQuery(params)}`,
      ));
    },

    upload: async (
      datasetId: string,
      files: File[],
    ): Promise<KnowledgeDocument[]> => {
      const formData = new FormData();
      for (const file of files) formData.append("files", file);
      const data = await unwrap<RawObject[]>(
        apiClient.post(
          `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents`,
          formData,
          {
            headers: { "Content-Type": "multipart/form-data" },
            timeout: DOCUMENT_UPLOAD_TIMEOUT_MS,
          },
        ),
      );
      return (data).map(normalizeDocument);
    },

    /** Metadata in parser_config requires a complete JSON Schema; partial schemas are rejected by the gateway. Prefer the dedicated metadata/config endpoint for template editing. */
    update: async (
      datasetId: string,
      documentId: string,
      patch: Record<string, unknown>,
    ): Promise<KnowledgeDocument> => {
      const data = await unwrap<RawObject>(
        apiClient.put(
          `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}`,
          patch,
        ),
      );
      return normalizeDocument(data);
    },

    remove: (datasetId: string, documentIds: string[]): Promise<unknown> =>
      apiClient.delete(
        `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents`,
        {
          data: { ids: documentIds },
        },
      ),

    parse: (datasetId: string, documentIds: string[]): Promise<unknown> =>
      apiClient.post(
        `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/parse`,
        { document_ids: documentIds },
      ),

    stopParse: (datasetId: string, documentIds: string[]): Promise<unknown> =>
      apiClient.delete(
        `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/parse`,
        {
          data: { document_ids: documentIds },
        },
      ),

    ingest: async (input: DocumentIngestInput): Promise<true> => {
      const accepted = await unwrap<unknown>(apiClient.post(`${BASE}/documents/ingest`, input));
      if (accepted !== true) throw new Error(i18next.t("knowledge.manage.ingestNotAccepted"));
      return true;
    },

    downloadUrl: (datasetId: string, documentId: string): string =>
      `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/download`,

    download: (
      datasetId: string,
      documentId: string,
    ): Promise<AxiosResponse<Blob>> =>
      apiClient.get<Blob>(
        knowledgeApi.documents.downloadUrl(datasetId, documentId),
        {
          responseType: "blob",
          headers: { Accept: "application/octet-stream, */*" },
          timeout: 120000,
        },
      ),
  },

  images: {
    url: (datasetId: string, imageId: string): string =>
      `${BASE}/datasets/${encodeURIComponent(datasetId)}/images/${encodeURIComponent(imageId)}`,
    fetch: async (
      datasetId: string,
      imageId: string,
      signal?: AbortSignal,
    ): Promise<Blob> => {
      const response = await apiClient.get<Blob>(
        knowledgeApi.images.url(datasetId, imageId),
        {
          responseType: "blob",
          headers: { Accept: "image/*" },
          signal,
        },
      );
      return response.data;
    },
  },

  chunks: {
    list: async (
      datasetId: string,
      documentId: string,
      params: ChunkListParams = {},
    ): Promise<ChunkListResult> => {
      const qs = buildQuery({
        id: params.id,
        page: params.page,
        page_size: params.page_size,
        keywords: params.keywords,
        available: params.available,
      });
      const data = await unwrap<{
        total: number;
        chunks: RawObject[];
        document?: RawObject;
      }>(
        apiClient.get(
          `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/chunks${qs}`,
        ),
      );
      return {
        total: num(data.total),
        chunks: (data.chunks).map(normalizeChunk),
        document: data.document ? normalizeDocument(data.document) : null,
      };
    },

    create: async (
      datasetId: string,
      documentId: string,
      input: ChunkFormInput,
    ): Promise<KnowledgeChunk> => {
      const data = await unwrap<RawObject>(
        apiClient.post(
          `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/chunks`,
          toChunkBody(input),
        ),
      );
      return normalizeChunk(data);
    },

    update: async (
      datasetId: string,
      documentId: string,
      chunkId: string,
      input: ChunkFormInput,
    ): Promise<KnowledgeChunk> => {
      const data = await unwrap<RawObject>(
        apiClient.put(
          `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/chunks/${encodeURIComponent(chunkId)}`,
          toChunkBody(input),
        ),
      );
      return normalizeChunk(data);
    },

    remove: (
      datasetId: string,
      documentId: string,
      chunkIds: string[],
    ): Promise<unknown> =>
      apiClient.delete(
        `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/chunks`,
        { data: { chunk_ids: chunkIds } },
      ),

    switch: (
      datasetId: string,
      documentId: string,
      chunkIds: string[],
      available: boolean,
    ): Promise<unknown> =>
      apiClient.post(
        `${BASE}/datasets/${encodeURIComponent(datasetId)}/documents/${encodeURIComponent(documentId)}/chunks/switch`,
        { chunk_ids: chunkIds, available },
      ),
  },

  retrieval: {
    test: async (input: RetrievalInput): Promise<RetrievalResult> => {
      const data = await unwrap<RawObject>(
        apiClient.post(`${BASE}/retrieval`, input),
      );
      return normalizeRetrievalResult(data);
    },
  },
};
