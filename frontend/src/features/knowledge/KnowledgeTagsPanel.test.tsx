import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, within, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { ConfigProvider } from "antd";
import axios, { AxiosError } from "axios";
import apiClient, {
  getAccessToken,
  getRefreshToken,
  setTokens,
} from "@/api/client";
import KnowledgeTagsPanel from "./KnowledgeTagsPanel";
import { setAuthRole } from "@/test/auth-store-mock";
const h = vi.hoisted(() => ({
  request: vi.fn(),
  aggregate: vi.fn(),
  realAggregate: undefined as
    | typeof import("@/api/knowledgeManagement").knowledgeTagsApi.aggregate
    | undefined,
  token: "tag-token",
  accountId: "1",
  sourceError: null as unknown,
  pages: [[{ id: "source", name: "Source library" }]] as {
    id: string;
    name: string;
  }[][],
  total: 1,
  origin: undefined as { id: string; token: string; role: string } | undefined,
  update: vi.fn(),
  get: vi.fn(),
  refetch: vi.fn(),
  config: {} as Record<string, unknown>,
}));
vi.mock("@/stores/auth", async () => {
  const { getAuthUser } = await import("@/test/auth-store-mock");
  const getState = () => ({ user: { ...getAuthUser(), id: h.accountId } });
  return {
    useAuthStore: Object.assign(
      (selector: (state: ReturnType<typeof getState>) => unknown) =>
        selector(getState()),
      { getState },
    ),
  };
});
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeResource: () => ({
    data: [["manual", 3]],
    refetch: h.refetch,
    isFetching: false,
    error: null,
  }),
}));
vi.mock("@/queries/useKnowledge", () => ({
  useKnowledgeDetail: () => ({
    data: { parser_config: h.config },
    refetch: h.refetch,
    error: null,
  }),
  useKnowledgeList: (params: { page: number }) => ({
    data: { datasets: h.pages[params.page - 1] ?? [], total: h.total },
    origin: h.origin,
    isFetching: false,
    error: h.sourceError,
    refetch: h.refetch,
  }),
}));
vi.mock("@/api/knowledgeManagement", async (original) => {
  const actual = await original<typeof import("@/api/knowledgeManagement")>();
  h.realAggregate = actual.knowledgeTagsApi.aggregate;
  return {
    ...actual,
    knowledgeRequest: h.request,
    knowledgeTagsApi: { aggregate: h.aggregate },
  };
});
vi.mock("@/api/knowledge", () => ({
  knowledgeApi: { datasets: { update: h.update, get: h.get } },
}));
beforeEach(() => {
  setAuthRole("admin");
  h.accountId = "1";
  h.token = "tag-token";
  localStorage.setItem("access_token", h.token);
  h.origin = { id: "1", token: h.token, role: "admin" };
  h.pages = [[{ id: "source", name: "Source library" }]];
  h.total = 1;
  h.sourceError = null;
  h.aggregate.mockReset();
  h.aggregate.mockResolvedValue([]);
  h.config = {
    tag_kb_ids: ["source"],
    topn_tags: 2,
    unrelated: { keep: false },
  };
  h.request.mockReset();
  h.update.mockReset();
  h.get.mockReset();
  h.refetch.mockReset();
  h.request.mockResolvedValue([]);
  h.get.mockResolvedValue({ parser_config: h.config });
});
it("confirms delete impact and retries failed readback without resending deletion", async () => {
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeTagsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "删除" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(/当前使用次数：3/)).toBeInTheDocument();
  expect(h.request).not.toHaveBeenCalled();
  h.request.mockImplementation(async (method) =>
    method === "get" ? [["manual", 3]] : true,
  );
  await user.click(within(dialog).getByRole("button", { name: /删\s*除/ }));
  await within(dialog).findByText("保存后回读未确认，请重试");
  h.request.mockResolvedValue([]);
  await user.click(within(dialog).getByRole("button", { name: "仅重试读回" }));
  await screen.findByText("标签修改已读回确认。");
  expect(
    h.request.mock.calls.filter(([method]) => method === "delete"),
  ).toHaveLength(1);
});
it("submits only touched tag configuration, preserving omission of the source list", async () => {
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeTagsPanel id="kb1" />
    </ConfigProvider>,
  );
  const count = screen.getByRole("spinbutton");
  await user.clear(count);
  await user.type(count, "4");
  h.get.mockResolvedValue({ parser_config: { ...h.config, topn_tags: 4 } });
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("设置已保存并读回确认。");
  expect(h.update).toHaveBeenCalledWith("kb1", {
    parser_config: { topn_tags: 4 },
  });
});
it("saves an explicitly cleared tag source list as [] and preserves the draft on stale readback", async () => {
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeTagsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByLabelText("close-circle"));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("保存后回读未确认，请重试");
  expect(h.update).toHaveBeenCalledWith("kb1", {
    parser_config: { tag_kb_ids: [] },
  });
  expect(
    screen.queryByText("Source library", {
      selector: ".ant-select-selection-item-content",
    }),
  ).not.toBeInTheDocument();
});
it("hides write operations from members", () => {
  setAuthRole("member");
  render(
    <ConfigProvider>
      <KnowledgeTagsPanel id="kb1" />
    </ConfigProvider>,
  );
  expect(
    screen.queryByRole("button", { name: "删除" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /保\s*存/ }),
  ).not.toBeInTheDocument();
});
it("does not confirm deletion from a capped tag aggregation and retries readback without another delete", async () => {
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeTagsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "删除" }));
  const dialog = await screen.findByRole("dialog");
  const capped = Array.from({ length: 1000 }, (_, index) => [
    `tag-${index}`,
    2,
  ]);
  h.request.mockImplementation(async (method) =>
    method === "get" ? capped : true,
  );
  await user.click(within(dialog).getByRole("button", { name: /删\s*除/ }));
  await within(dialog).findByText(/标签读回达到 1000 项聚合上限/);
  expect(screen.queryByText("标签修改已读回确认。")).not.toBeInTheDocument();
  h.request.mockResolvedValue([]);
  await user.click(within(dialog).getByRole("button", { name: "仅重试读回" }));
  await screen.findByText("标签修改已读回确认。");
  expect(
    h.request.mock.calls.filter(([method]) => method === "delete"),
  ).toHaveLength(1);
});

async function openAggregation() {
  const user = userEvent.setup();
  const view = render(
    <MemoryRouter>
      <ConfigProvider>
        <KnowledgeTagsPanel id="kb1" />
      </ConfigProvider>
    </MemoryRouter>,
  );
  await user.click(screen.getByText("跨知识库标签分布"));
  return { user, ...view };
}
async function selectSource(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  const row = screen.getByText(name, { selector: "td span" }).closest("tr")!;
  await user.click(within(row).getByRole("checkbox"));
}
it("keeps explicit selection across pages and reports combined and observed per-library counts", async () => {
  h.pages = [
    [{ id: "a", name: "Library A" }],
    [{ id: "b", name: "Library B" }],
  ];
  h.total = 11;
  h.aggregate.mockImplementation(async (ids: string[]) =>
    ids.length === 2
      ? [
          { value: "shared", count: 7 },
          { value: "only-a", count: 0 },
        ]
      : ids[0] === "a"
        ? [
            { value: "shared", count: 2 },
            { value: "only-a", count: 0 },
          ]
        : [
            { value: "shared", count: 5 },
            { value: "only-b", count: 4 },
          ],
  );
  const { user } = await openAggregation();
  expect(screen.getByRole("button", { name: "查询已选知识库" })).toBeDisabled();
  await selectSource(user, "Library A");
  await user.click(screen.getByTitle("2"));
  await selectSource(user, "Library B");
  await user.click(screen.getByRole("button", { name: "查询已选知识库" }));
  await screen.findByText("本次查询范围：Library A / Library B");
  expect(h.aggregate.mock.calls.map(([ids]) => ids)).toEqual([
    ["a", "b"],
    ["a"],
    ["b"],
  ]);
  const row = screen.getByText("shared", { selector: "span" }).closest("tr")!;
  expect(
    within(row)
      .getAllByRole("cell")
      .map((cell) => cell.textContent),
  ).toEqual(["shared", "7", "2", "5"]);
  const missing = screen
    .getByText("only-a", { selector: "span" })
    .closest("tr")!;
  expect(
    within(missing)
      .getAllByRole("cell")
      .map((cell) => cell.textContent),
  ).toEqual(["only-a", "0", "0", "未返回"]);
  const libraryOnly = screen
    .getByText("only-b", { selector: "span" })
    .closest("tr")!;
  expect(
    within(libraryOnly)
      .getAllByRole("cell")
      .map((cell) => cell.textContent),
  ).toEqual(["only-b", "未返回", "未返回", "4"]);
  expect(
    screen.getByText("已返回的标签计数不代表完整标签清单。"),
  ).toBeInTheDocument();
  expect(
    screen.getAllByRole("link", { name: "知识库设置" })[0],
  ).toHaveAttribute("href", "/knowledge/a/settings");
  expect(screen.getAllByRole("link", { name: "检索测试" })[1]).toHaveAttribute(
    "href",
    "/knowledge/b/retrieval",
  );
});
it("shows no partial distribution after a library fails and preserves the selected scope for retry", async () => {
  h.pages = [
    [
      { id: "a", name: "Library A" },
      { id: "b", name: "Library B" },
    ],
  ];
  h.total = 2;
  h.aggregate.mockImplementation(async (ids: string[]) => {
    if (ids.length === 1 && ids[0] === "b")
      throw new Error("Permission denied");
    return [{ value: "partial-tag", count: 2 }];
  });
  const { user } = await openAggregation();
  await selectSource(user, "Library A");
  await selectSource(user, "Library B");
  await user.click(screen.getByRole("button", { name: "查询已选知识库" }));
  await screen.findByText("Permission denied");
  expect(screen.queryByText("partial-tag")).not.toBeInTheDocument();
  expect(screen.getByText("已选 2 个知识库")).toBeInTheDocument();
  h.aggregate.mockResolvedValue([]);
  await user.click(screen.getByRole("button", { name: "查询已选知识库" }));
  await screen.findByText(
    "所选范围未返回标签，不能据此证明所有已选知识库都没有标签。",
  );
});
it("warns at the bucket cap without claiming complete results", async () => {
  h.aggregate.mockResolvedValue(
    Array.from({ length: 1000 }, (_, index) => ({
      value: `cap-${index}`,
      count: 1,
    })),
  );
  const { user } = await openAggregation();
  await selectSource(user, "Source library");
  await user.click(screen.getByRole("button", { name: "查询已选知识库" }));
  await screen.findByText(/某次响应已达到 1000 个标签桶/);
  expect(h.aggregate).toHaveBeenCalledTimes(1);
});
it("blocks old-token cached selection before userinfo changes and hides previous results", async () => {
  h.aggregate.mockResolvedValue([{ value: "private-tag", count: 1 }]);
  const { user } = await openAggregation();
  await selectSource(user, "Source library");
  await user.click(screen.getByRole("button", { name: "查询已选知识库" }));
  await screen.findByText("private-tag", { selector: "span" });
  act(() => {
    localStorage.setItem("access_token", "replacement-token");
    window.dispatchEvent(new Event("storage"));
  });
  await screen.findByText("账号或凭据已变化，请重新加载来源并重新选库。");
  expect(
    screen.queryByText("private-tag", { selector: "span" }),
  ).not.toBeInTheDocument();
  expect(screen.getByText("已选 0 个知识库")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "查询已选知识库" })).toBeDisabled();
  expect(h.aggregate).toHaveBeenCalledTimes(1);
});
it("does not publish an in-flight response or schedule library reads after account change", async () => {
  let finish!: (rows: { value: string; count: number }[]) => void;
  h.aggregate.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  h.pages = [
    [
      { id: "a", name: "Library A" },
      { id: "b", name: "Library B" },
    ],
  ];
  const { user, rerender } = await openAggregation();
  await selectSource(user, "Library A");
  await selectSource(user, "Library B");
  await user.click(screen.getByRole("button", { name: "查询已选知识库" }));
  h.accountId = "2";
  rerender(
    <MemoryRouter>
      <ConfigProvider>
        <KnowledgeTagsPanel id="kb1" />
      </ConfigProvider>
    </MemoryRouter>,
  );
  await act(async () => {
    finish([{ value: "old-account", count: 7 }]);
  });
  await waitFor(() =>
    expect(screen.queryByText("old-account")).not.toBeInTheDocument(),
  );
  expect(h.aggregate).toHaveBeenCalledTimes(1);
});
it("blocks a failed source page without expanding the previously selected scope", async () => {
  const { user, rerender } = await openAggregation();
  await selectSource(user, "Source library");
  h.sourceError = new Error("Page permission failure");
  rerender(
    <MemoryRouter>
      <ConfigProvider>
        <KnowledgeTagsPanel id="kb1" />
      </ConfigProvider>
    </MemoryRouter>,
  );
  await screen.findAllByText("Page permission failure");
  expect(screen.getByRole("button", { name: "查询已选知识库" })).toBeDisabled();
  expect(h.aggregate).not.toHaveBeenCalled();
});

it("allows members to query readonly distribution", async () => {
  setAuthRole("member");
  h.origin = { id: "1", token: h.token, role: "member" };
  const { user } = await openAggregation();
  await selectSource(user, "Source library");
  await user.click(screen.getByRole("button", { name: "查询已选知识库" }));
  await screen.findByText(
    "所选范围未返回标签，不能据此证明所有已选知识库都没有标签。",
  );
  expect(h.update).not.toHaveBeenCalled();
  expect(h.request).not.toHaveBeenCalled();
});
it("discards prior selection permanently when credentials switch away and back", async () => {
  const { user } = await openAggregation();
  await selectSource(user, "Source library");
  act(() => {
    localStorage.setItem("access_token", "another-token");
    window.dispatchEvent(new Event("storage"));
  });
  await screen.findByText("已选 0 个知识库");
  act(() => {
    localStorage.setItem("access_token", h.token);
    window.dispatchEvent(new Event("storage"));
  });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "查询已选知识库" }),
    ).toBeDisabled(),
  );
  expect(h.aggregate).not.toHaveBeenCalled();
});

it("keeps the C3 mount, scope and result through real 401 refresh with delayed identity verification", async () => {
  setTokens(h.token, "valid-refresh");
  h.aggregate.mockImplementation(h.realAggregate!);
  h.refetch.mockImplementation(async () => {
    h.origin = { id: "1", role: "admin", token: getAccessToken()! };
  });
  const original = apiClient.defaults.adapter;
  const refresh = vi.spyOn(axios, "post").mockResolvedValue({
    data: {
      success: true,
      data: { accessToken: "fresh-access", refreshToken: "fresh-refresh" },
    },
  });
  let finishIdentity!: () => void;
  const verified = new Promise<void>((resolve) => {
    finishIdentity = resolve;
  });
  apiClient.defaults.adapter = async (config) => {
    const expired = config.headers.get("Authorization") === `Bearer ${h.token}`;
    const response = {
      data: {
        success: !expired,
        data:
          config.url === "/auth/userinfo"
            ? { user_id: "1", roles: ["admin"] }
            : [{ value: "refreshed-tag", count: 9 }],
      },
      status: expired ? 401 : 200,
      statusText: "OK",
      headers: {},
      config,
    };
    if (expired && config.validateStatus && !config.validateStatus(401))
      throw new AxiosError(
        "Expired",
        AxiosError.ERR_BAD_REQUEST,
        config,
        undefined,
        response,
      );
    if (!expired && config.url === "/auth/userinfo") await verified;
    return response;
  };
  try {
    const { user } = await openAggregation();
    await selectSource(user, "Source library");
    await user.click(screen.getByRole("button", { name: "查询已选知识库" }));
    await waitFor(() => {
      expect(getAccessToken()).toBe("fresh-access");
    });
    // Exercise the parent's credential observer before userinfo resolves.
    act(() => {
      window.dispatchEvent(new Event("storage"));
    });
    await new Promise((resolve) => setTimeout(resolve, 300));
    await act(async () => {
      finishIdentity();
    });
    await screen.findByText("refreshed-tag", { selector: "span" });
    expect(screen.getByText("已选 1 个知识库")).toBeInTheDocument();
    expect(getAccessToken()).toBe("fresh-access");
    expect(getRefreshToken()).toBe("fresh-refresh");
    expect(refresh).toHaveBeenCalledTimes(1);
  } finally {
    apiClient.defaults.adapter = original;
    refresh.mockRestore();
  }
});
