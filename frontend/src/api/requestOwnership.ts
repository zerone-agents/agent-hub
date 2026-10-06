import apiClient from "./client";
import type { AxiosRequestConfig, AxiosRequestTransformer, AxiosResponseTransformer } from "axios";

export interface RequestOwner {
  isCurrent: () => boolean;
  assertCurrent: (credentialRefresh?: boolean) => void;
  signal: AbortSignal;
}

// Guard at dispatch/response transformation as well as between REST phases.
// Axios's token interceptor runs asynchronously; checking only before post()
// would leave a window in which another account's token could be attached.
export function ownedRequestConfig(owner?: RequestOwner): AxiosRequestConfig | undefined {
  if (!owner) return undefined;
  const requestDefaults = apiClient.defaults.transformRequest;
  const responseDefaults = apiClient.defaults.transformResponse;
  const requestGuard: AxiosRequestTransformer = function (data: unknown) {
    owner.assertCurrent(this.headers.get('X-Refresh-Attempt') === 'true');
    return data;
  };
  const responseGuard: AxiosResponseTransformer = (data: unknown) => { owner.assertCurrent(); return data; };
  return {
    signal: owner.signal,
    transformRequest: [requestGuard, ...(Array.isArray(requestDefaults) ? requestDefaults : requestDefaults ? [requestDefaults] : [])],
    transformResponse: [responseGuard, ...(Array.isArray(responseDefaults) ? responseDefaults : responseDefaults ? [responseDefaults] : [])],
  };
}
