import { afterEach, expect, it, vi } from "vitest";
import apiClient, {
  getAccessToken,
  getRefreshToken,
  setTokens,
} from "./client";
import axios, { AxiosError } from "axios";
import { useAuthStore } from "@/stores/auth";
import {
  knowledgeMetadataApi,
  knowledgeTagsApi,
  createKnowledgeTagOwner,
} from "./knowledgeManagement";

it("uses global metadata routes with explicit datasets and rejects business failures", async () => {
  const get = vi
    .spyOn(apiClient, "get")
    .mockResolvedValueOnce({ data: { success: true, data: ["version"] } })
    .mockResolvedValueOnce({
      data: { success: true, data: { version: ["v2"] } },
    })
    .mockResolvedValueOnce({
      data: { success: false, error: "No authorization" },
    });
  await expect(knowledgeMetadataApi.keys(["kb1", "kb2"])).resolves.toEqual([
    "version",
  ]);
  await expect(knowledgeMetadataApi.flattened(["kb1"])).resolves.toEqual({
    version: ["v2"],
  });
  expect(get).toHaveBeenCalledWith(
    "/api/v1/admin/knowledge/datasets/metadata/keys",
    { params: { dataset_ids: "kb1,kb2" } },
  );
  expect(get).toHaveBeenCalledWith(
    "/api/v1/admin/knowledge/datasets/metadata/flattened",
    { params: { dataset_ids: "kb1" } },
  );
  await expect(knowledgeMetadataApi.keys(["kb1"])).rejects.toThrow(
    "No authorization",
  );
  get.mockRestore();
});

const controller = new AbortController();
const owner = {
  signal: controller.signal,
  isCurrent: () => true,
  assertCurrent: vi.fn(),
  verifyCredentials: vi.fn().mockResolvedValue(undefined),
};
afterEach(() => {
  vi.restoreAllMocks();
  owner.assertCurrent.mockReset();
});
it("uses explicit normalized tag aggregation scope and preserves zero counts", async () => {
  const get = vi.spyOn(apiClient, "get").mockResolvedValue({
    data: { success: true, data: [{ value: "tag", count: 0 }] },
  });
  await expect(
    knowledgeTagsApi.aggregate([" kb1 ", "kb2", "kb1"], owner),
  ).resolves.toEqual([{ value: "tag", count: 0 }]);
  expect(get).toHaveBeenCalledWith(
    "/api/v1/admin/knowledge/datasets/tags/aggregation",
    expect.objectContaining({
      params: { dataset_ids: "kb1,kb2" },
      signal: controller.signal,
    }),
  );
  expect(owner.assertCurrent).toHaveBeenCalledTimes(2);
});
it.each([
  { ids: [] },
  { ids: [""] },
  { ids: ["a,b"] },
  { ids: ["a/b"] },
  { ids: ["a".repeat(33)] },
])(
  "rejects empty or invalid tag scope $ids before dispatch",
  async ({ ids }) => {
    const get = vi.spyOn(apiClient, "get");
    await expect(
      knowledgeTagsApi.aggregate(ids as string[], owner),
    ).rejects.toThrow();
    expect(get).not.toHaveBeenCalled();
  },
);
it.each(
  [
    null,
    [["tag", 3]],
    [{ value: "tag", count: -1 }],
    [{ value: "tag", count: 1.5 }],
    [{ value: "tag", count: Number.MAX_SAFE_INTEGER + 1 }],
    [{ value: "tag", count: "3" }],
    [
      { value: "tag", count: 1 },
      { value: "tag", count: 2 },
    ],
  ].map((data) => ({ data })),
)("rejects malformed aggregation $data", async ({ data }) => {
  vi.spyOn(apiClient, "get").mockResolvedValue({
    data: { success: true, data },
  });
  await expect(knowledgeTagsApi.aggregate(["kb1"], owner)).rejects.toThrow();
});
it("rejects business failure and owner changes after readback", async () => {
  vi.spyOn(apiClient, "get")
    .mockResolvedValueOnce({
      data: { success: false, error: "No authorization" },
    })
    .mockResolvedValueOnce({ data: { success: true, data: [] } });
  await expect(knowledgeTagsApi.aggregate(["kb1"], owner)).rejects.toThrow(
    "No authorization",
  );
  owner.assertCurrent
    .mockImplementationOnce(() => undefined)
    .mockImplementationOnce(() => {
      throw new Error("Owner changed");
    });
  await expect(knowledgeTagsApi.aggregate(["kb1"], owner)).rejects.toThrow(
    "Owner changed",
  );
});

it("guards dispatch when credentials change before the asynchronous token interceptor", async () => {
  const adapter = vi.fn().mockResolvedValue({
    data: { success: true, data: [] },
    status: 200,
    statusText: "OK",
    headers: {},
    config: {},
  });
  const original = apiClient.defaults.adapter;
  apiClient.defaults.adapter = adapter;
  let current = true;
  const guarded = {
    signal: new AbortController().signal,
    verifyCredentials: vi.fn().mockResolvedValue(undefined),
    isCurrent: () => current,
    assertCurrent: () => {
      if (!current) throw new Error("Owner changed");
    },
  };
  try {
    const promise = knowledgeTagsApi.aggregate(["kb1"], guarded);
    current = false;
    await expect(promise).rejects.toThrow("Owner changed");
    expect(adapter).not.toHaveBeenCalled();
  } finally {
    apiClient.defaults.adapter = original;
  }
});
it("guards response transformation when credentials change during network IO", async () => {
  const original = apiClient.defaults.adapter;
  let current = true;
  apiClient.defaults.adapter = async (config) => {
    current = false;
    return {
      data: { success: true, data: [{ value: "private", count: 1 }] },
      status: 200,
      statusText: "OK",
      headers: {},
      config,
    };
  };
  const guarded = {
    signal: new AbortController().signal,
    verifyCredentials: vi.fn().mockResolvedValue(undefined),
    isCurrent: () => current,
    assertCurrent: () => {
      if (!current) throw new Error("Owner changed");
    },
  };
  try {
    await expect(knowledgeTagsApi.aggregate(["kb1"], guarded)).rejects.toThrow(
      "Owner changed",
    );
  } finally {
    apiClient.defaults.adapter = original;
  }
});

it.each([
  "same-account",
  "foreign-token",
  "changed-store",
  "cancelled",
  "role-changed",
  "second-401",
])(
  "handles 401 refresh safely for %s before replaying selected dataset IDs",
  async (scenario) => {
    useAuthStore
      .getState()
      .setUser({ id: "A", name: "A", email: "a@example.test", role: "admin" });
    setTokens("expired-access", "valid-refresh");
    const scopeOrigin = { id: "A", role: "admin", token: "expired-access" };
    const callbacks = { refreshing: vi.fn(), verified: vi.fn() };
    const controller = new AbortController();
    const guarded = createKnowledgeTagOwner(
      { ...scopeOrigin },
      controller.signal,
      callbacks,
    );
    const calls: { url: string; token: string; ids?: string }[] = [];
    const refresh = vi.spyOn(axios, "post").mockImplementation(async () => {
      if (scenario === "changed-store")
        useAuthStore.getState().setUser({
          id: "B",
          name: "B",
          email: "b@example.test",
          role: "admin",
        });
      if (scenario === "cancelled") controller.abort();
      return {
        data: {
          success: true,
          data: { accessToken: "fresh-access", refreshToken: "fresh-refresh" },
        },
      };
    });
    const original = apiClient.defaults.adapter;
    apiClient.defaults.adapter = async (config) => {
      const authorization = config.headers.get("Authorization");
      const token = typeof authorization === "string" ? authorization : "";
      calls.push({
        url: config.url ?? "",
        token,
        ids: config.params?.dataset_ids,
      });
      const status =
        token === "Bearer expired-access" ||
        (scenario === "second-401" && config.url?.endsWith("/aggregation"))
          ? 401
          : 200;
      const data =
        config.url === "/auth/userinfo"
          ? {
              user_id: scenario === "foreign-token" ? "B" : "A",
              roles: [scenario === "role-changed" ? "member" : "admin"],
            }
          : [{ value: "safe-tag", count: 7 }];
      const response = {
        data: { success: status === 200, data },
        status,
        statusText: status === 200 ? "OK" : "Unauthorized",
        headers: {},
        config,
      };
      // Mirror Axios adapter status settlement: C3 accepts aggregate 401 for
      // scope-free revalidation; userinfo 401 goes through the shared refresh.
      if (config.validateStatus && !config.validateStatus(status))
        throw new AxiosError(
          "Expired",
          AxiosError.ERR_BAD_REQUEST,
          config,
          undefined,
          response,
        );
      return response;
    };
    try {
      const query = knowledgeTagsApi.aggregate(["a", "b"], guarded);
      if (scenario === "same-account") {
        await expect(query).resolves.toEqual([{ value: "safe-tag", count: 7 }]);
        expect(callbacks.verified).toHaveBeenCalledWith({
          id: "A",
          role: "admin",
          token: "fresh-access",
        });
      } else await expect(query).rejects.toThrow();
      expect(refresh).toHaveBeenCalledTimes(1);
      expect(getAccessToken()).toBe("fresh-access");
      expect(getRefreshToken()).toBe("fresh-refresh");
      expect(scopeOrigin.token).toBe("expired-access");
      const scopeCalls = calls.filter((call) => call.ids);
      expect(scopeCalls).toEqual(
        scenario === "same-account" || scenario === "second-401"
          ? [
              {
                url: "/api/v1/admin/knowledge/datasets/tags/aggregation",
                token: "Bearer expired-access",
                ids: "a,b",
              },
              {
                url: "/api/v1/admin/knowledge/datasets/tags/aggregation",
                token: "Bearer fresh-access",
                ids: "a,b",
              },
            ]
          : [
              {
                url: "/api/v1/admin/knowledge/datasets/tags/aggregation",
                token: "Bearer expired-access",
                ids: "a,b",
              },
            ],
      );
      if (scenario !== "same-account" && scenario !== "second-401")
        expect(callbacks.verified).not.toHaveBeenCalled();
      expect(
        calls
          .filter((call) => call.url === "/auth/userinfo")
          .every((call) => !call.ids),
      ).toBe(true);
    } finally {
      apiClient.defaults.adapter = original;
      localStorage.clear();
    }
  },
);
