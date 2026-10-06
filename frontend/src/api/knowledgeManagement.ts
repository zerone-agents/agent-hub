import apiClient, { getAccessToken, unwrapResponse } from "./client";
import { useAuthStore } from "@/stores/auth";
import type { UserInfoResponse } from "./auth";
import i18next from "@/i18n";
import { ownedRequestConfig, type RequestOwner } from "./requestOwnership";
import { normalizeDocument, type KnowledgeDocument } from "./knowledge";

export type KnowledgeObject = Record<string, unknown>;
export interface KnowledgeIndexCancelResult {
  task_id: string;
  /** Upstream accepted the request; this does not prove worker termination. */
  request_accepted: boolean;
  /** True only when readback contains the new cancellation marker. */
  cancel_requested: boolean;
  task: KnowledgeObject;
}
interface Envelope<T> {
  success: boolean;
  data: T;
  error?: string;
}
export async function knowledgeRequest<T>(
  method: "get" | "post" | "put" | "patch" | "delete",
  datasetId: string,
  resource: string,
  data?: unknown,
  params?: KnowledgeObject,
): Promise<T> {
  const response = await apiClient.request<Envelope<T>>({
    method,
    url: `/api/v1/admin/knowledge/datasets/${encodeURIComponent(datasetId)}/${resource}`,
    data,
    params,
    timeout: resource === "documents/create-web" ? 60 * 60 * 1000 : undefined,
  });
  if (!response.data.success) throw new Error(response.data.error);
  return response.data.data;
}
export const knowledgeManagement = {
  documentStatus: (datasetId: string, ids: string[], enabled: boolean) =>
    knowledgeRequest<{ succeeded: string[]; failed: string[] }>(
      "post",
      datasetId,
      "documents/batch-update-status",
      { doc_ids: ids, status: enabled ? "1" : "0" },
    ),
  createDocument: async (
    datasetId: string,
    mode: "web" | "empty",
    name: string,
    url?: string,
  ): Promise<KnowledgeDocument> =>
    normalizeDocument(
      await knowledgeRequest<KnowledgeObject>(
        "post",
        datasetId,
        `documents/create-${mode}`,
        { name, url },
      ),
    ),
  documentMetadataConfig: (
    datasetId: string,
    documentId: string,
    metadata: unknown,
  ) =>
    knowledgeRequest(
      "put",
      datasetId,
      `documents/${encodeURIComponent(documentId)}/metadata/config`,
      { metadata },
    ),
};

export function parseObjectJSON(value: string): KnowledgeObject {
  const object: unknown = JSON.parse(value);
  if (object === null || typeof object !== "object" || Array.isArray(object))
    throw new Error("JSON object required");
  return object as KnowledgeObject;
}

export function containsKnowledgeFields(
  actual: unknown,
  expected: unknown,
): boolean {
  if (Array.isArray(expected))
    return (
      Array.isArray(actual) &&
      actual.length === expected.length &&
      expected.every((value: unknown, index) =>
        containsKnowledgeFields(actual[index], value),
      )
    );
  if (expected !== null && typeof expected === "object") {
    if (actual === null || typeof actual !== "object" || Array.isArray(actual))
      return false;
    const values = actual as KnowledgeObject;
    return Object.entries(expected).every(([key, value]) =>
      containsKnowledgeFields(values[key], value),
    );
  }
  return actual === expected;
}

export const knowledgeMetadataApi = {
  keys: async (datasetIds: string[]): Promise<string[]> => {
    const response = await apiClient.get<Envelope<string[]>>(
      "/api/v1/admin/knowledge/datasets/metadata/keys",
      { params: { dataset_ids: datasetIds.join(",") } },
    );
    if (!response.data.success) throw new Error(response.data.error);
    return response.data.data;
  },
  flattened: async (datasetIds: string[]): Promise<KnowledgeObject> => {
    const response = await apiClient.get<Envelope<KnowledgeObject>>(
      "/api/v1/admin/knowledge/datasets/metadata/flattened",
      { params: { dataset_ids: datasetIds.join(",") } },
    );
    if (!response.data.success) throw new Error(response.data.error);
    return response.data.data;
  },
};

export interface KnowledgeTagOrigin {
  id: string;
  token: string | null;
  role?: string;
}
export interface KnowledgeTagRequestOwner extends RequestOwner {
  verifyCredentials: () => Promise<void>;
}

/** Revalidate refreshed credentials without ever replaying dataset IDs first. */
export function createKnowledgeTagOwner(
  origin: KnowledgeTagOrigin,
  signal: AbortSignal,
  callbacks: {
    refreshing: (origin: KnowledgeTagOrigin) => void;
    verified: (origin: KnowledgeTagOrigin) => void;
  },
): KnowledgeTagRequestOwner {
  let verifiedToken = origin.token;
  const accountCurrent = () => {
    const user = useAuthStore.getState().user;
    return (
      !signal.aborted &&
      user?.id === origin.id &&
      user.role === origin.role &&
      origin.role !== "guest"
    );
  };
  const changed = () =>
    new Error(i18next.t("knowledgeMetadata:tagAggregationOwnerChanged"));
  const owner: KnowledgeTagRequestOwner = {
    signal,
    isCurrent: () => accountCurrent() && getAccessToken() === verifiedToken,
    assertCurrent: () => {
      if (!owner.isCurrent()) throw changed();
    },
    verifyCredentials: async () => {
      owner.assertCurrent();
      let identityToken = verifiedToken;
      let refreshing = false;
      const identityOwner: RequestOwner = {
        // Finish scope-free identity refresh even if the dataset operation is
        // cancelled. Reject account changes below, outside the client's refresh
        // catch, which otherwise clears another account's valid credentials.
        signal: new AbortController().signal,
        isCurrent: () => refreshing || owner.isCurrent(),
        assertCurrent: (refresh) => {
          if (refresh) {
            refreshing = true;
            identityToken = getAccessToken();
            if (accountCurrent())
              callbacks.refreshing({ ...origin, token: identityToken });
          }
          if (!identityOwner.isCurrent()) throw changed();
        },
      };
      const response = await apiClient.get(
        "/auth/userinfo",
        ownedRequestConfig(identityOwner),
      );
      if (!accountCurrent() || getAccessToken() !== identityToken)
        throw changed();
      const identity = unwrapResponse<UserInfoResponse>(response);
      const roles = identity.roles ?? [];
      const role = roles.includes("guest") ? "guest" : roles[0];
      if (
        (identity.user_id ?? identity.id) !== origin.id ||
        role !== origin.role
      )
        throw changed();
      verifiedToken = identityToken;
      origin.token = verifiedToken; // Operation-owned copy; never restamp cached list data.
      callbacks.verified({ ...origin });
    },
  };
  return owner;
}

export interface KnowledgeTagCount {
  value: string;
  count: number;
}

// Separate from the legacy per-dataset [tag, count] response.
export const knowledgeTagsApi = {
  aggregate: async (
    datasetIds: string[],
    owner: KnowledgeTagRequestOwner,
  ): Promise<KnowledgeTagCount[]> => {
    const ids = [...new Set(datasetIds.map((id) => id.trim()))];
    if (!ids.length || ids.some((id) => !/^[A-Za-z0-9_-]{1,32}$/.test(id)))
      throw new Error(
        i18next.t("knowledgeMetadata:tagAggregationInvalidScope"),
      );
    owner.assertCurrent();
    const read = () =>
      apiClient.get<Envelope<unknown>>(
        "/api/v1/admin/knowledge/datasets/tags/aggregation",
        {
          ...ownedRequestConfig(owner),
          params: { dataset_ids: ids.join(",") },
          // A dataset replay must wait for server identity verification. Let the
          // existing client refresh only the scope-free userinfo request below.
          validateStatus: (status) =>
            status === 401 || (status >= 200 && status < 300),
        },
      );
    let response = await read();
    if (response.status === 401) {
      await owner.verifyCredentials();
      owner.assertCurrent();
      response = await read();
      if (response.status === 401)
        throw new Error(i18next.t("apiErrors.unauthorized"));
    }
    owner.assertCurrent();
    if (!response.data.success) throw new Error(response.data.error);
    const rows = response.data.data;
    const seen = new Set<string>();
    if (
      !Array.isArray(rows) ||
      rows.some((row: unknown) => {
        if (!row || typeof row !== "object") return true;
        const { value, count } = row as Partial<KnowledgeTagCount>;
        if (
          typeof value !== "string" ||
          typeof count !== "number" ||
          !Number.isSafeInteger(count) ||
          count < 0 ||
          seen.has(value)
        )
          return true;
        seen.add(value);
        return false;
      })
    )
      throw new Error(
        i18next.t("knowledgeMetadata:tagAggregationInvalidResponse"),
      );
    return rows as KnowledgeTagCount[];
  },
};
