import type { UseQueryResult } from "@tanstack/react-query";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { message } from "antd";
import { parseApiError } from "@/api/client";
import {
  knowledgeRequest,
  knowledgeMetadataApi,
  type KnowledgeObject,
} from "@/api/knowledgeManagement";
import { knowledgeKeys } from "./useKnowledge";

export function useKnowledgeResource<T>(
  id: string,
  resource: string,
  params?: KnowledgeObject,
  poll = false,
): UseQueryResult<T> {
  return useQuery({
    queryKey: [...knowledgeKeys.all, "management", id, resource, params],
    queryFn: () => knowledgeRequest<T>("get", id, resource, undefined, params),
    enabled: Boolean(id),
    refetchInterval: poll
      ? (query) => {
          const task = query.state.data as KnowledgeObject | undefined;
          return task?.id && task.progress !== 1 && task.progress !== -1
            ? 3500
            : false;
        }
      : false,
  });
}
export function useKnowledgeAction(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      method,
      resource,
      data,
      params,
    }: {
      method: "post" | "put" | "patch" | "delete";
      resource: string;
      data?: unknown;
      params?: KnowledgeObject;
    }) => knowledgeRequest<unknown>(method, id, resource, data, params),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: knowledgeKeys.all });
    },
    onError: (error) => {
      message.error(parseApiError(error));
    },
  });
}

export function useKnowledgeMetadataInventory(id: string) {
  return useQuery({
    queryKey: [...knowledgeKeys.all, "metadata-inventory", id],
    queryFn: async () => {
      const [keys, flattened] = await Promise.all([
        knowledgeMetadataApi.keys([id]),
        knowledgeMetadataApi.flattened([id]),
      ]);
      return { keys, flattened };
    },
    enabled: Boolean(id),
  });
}
