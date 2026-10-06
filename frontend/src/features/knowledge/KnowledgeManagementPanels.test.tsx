import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import KnowledgeMetadataPanel from "./KnowledgeMetadataPanel";
import KnowledgeIngestionPanel from "./KnowledgeIngestionPanel";
import KnowledgeConnectorsPanel from "./KnowledgeConnectorsPanel";
import { setAuthRole } from "@/test/auth-store-mock";

const h = vi.hoisted(() => ({
  mutate: vi.fn(),
  mutateAsync: vi.fn(),
  refetch: vi.fn(),
  readback: vi.fn(),
  metadataConfig: {} as Record<string, unknown>,
}));
vi.mock("@/stores/auth", async () =>
  (await import("@/test/auth-store-mock")).createAuthStoreMock(),
);
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeResource: (_id: string, resource: string) => ({
    data:
      resource === "metadata/config"
        ? h.metadataConfig
        : resource === "metadata/summary"
          ? { summary: { author: [["Alice", 2]] } }
          : resource === "index"
            ? { id: "task1", progress: 1 }
            : resource === "graph/search"
              ? { graph: { nodes: [], edges: [] } }
              : resource === "sources" || resource === "connectors"
                ? []
                : undefined,
    isFetching: false,
    isLoading: false,
    error: null,
    refetch: h.refetch,
  }),
  useKnowledgeAction: () => ({
    mutate: h.mutate,
    mutateAsync: h.mutateAsync,
    isPending: false,
    isSuccess: false,
  }),
}));
vi.mock("@/api/knowledgeManagement", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/knowledgeManagement")>()),
  knowledgeRequest: h.readback,
}));
function renderPanel(element: React.ReactNode) {
  return render(<ConfigProvider>{element}</ConfigProvider>);
}
beforeEach(() => {
  h.metadataConfig = { enabled: true, fields: [{ key: "author", enum: [] }] };
  setAuthRole("admin");
  h.mutate.mockReset();
  h.mutateAsync.mockReset();
  h.refetch.mockReset();
  h.readback.mockReset();
});
describe("knowledge management panels", () => {
  it("skips unchanged metadata and saves edited fields with independent readback", async () => {
    h.readback.mockImplementation(async (method, _id, _resource, body) => {
      if (method === "put") {
        h.metadataConfig = { ...h.metadataConfig, ...body };
        return true;
      }
      return h.metadataConfig;
    });
    const user = userEvent.setup();
    renderPanel(<KnowledgeMetadataPanel id="kb1" />);
    expect(screen.getByText('"Alice"')).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    expect(h.readback).not.toHaveBeenCalled();
    const key = screen.getByRole("textbox", { name: "字段 1 的名称" });
    await user.clear(key);
    await user.type(key, "editor");
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await screen.findByText(/元数据配置已保存并回读确认/);
    expect(h.readback.mock.calls).toEqual([
      ["get", "kb1", "metadata/config"],
      [
        "put",
        "kb1",
        "metadata/config",
        {
          metadata: [{ key: "editor", enum: [] }],
        },
      ],
      ["get", "kb1", "metadata/config"],
    ]);
    expect(h.metadataConfig.enabled).toBe(true);
    expect(h.refetch).toHaveBeenCalled();
  });
  it("keeps index and metadata writes hidden for members", async () => {
    setAuthRole("member");
    renderPanel(
      <>
        <KnowledgeMetadataPanel id="kb1" />
        <KnowledgeIngestionPanel id="kb1" />
      </>,
    );
    expect(
      screen.queryByRole("button", { name: /保\s*存/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "运行索引" }),
    ).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: /库级索引任务/ }));
    expect(screen.queryByRole("button", { name: "运行索引" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "请求取消" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "删除索引产物" })).not.toBeInTheDocument();
    expect(screen.getAllByText("已完成").length).toBe(3);
  });
  it("recovers a created connector after linking fails without duplicating it", async () => {
    let linkCalls = 0;
    h.mutateAsync.mockImplementation(
      async ({ method, resource }: { method: string; resource: string }) => {
        if (method === "put" && ++linkCalls === 1)
          throw new Error("link failed");
        return resource === "sources" ? "source1" : true;
      },
    );
    h.readback.mockResolvedValue({
      id: "source1",
      config: { sync_deleted_files: false },
      refresh_freq: 60,
      prune_freq: 0,
      timeout_secs: 3600,
    });
    const user = userEvent.setup();
    renderPanel(<KnowledgeConnectorsPanel id="kb1" />);
    await user.click(screen.getByRole("button", { name: "创建数据源连接器" }));
    await user.type(screen.getByRole("textbox", { name: "名称" }), "S3 source");
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await screen.findByText(/数据源已创建。重试会保存/);
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    const requests = h.mutateAsync.mock.calls.map(
      (call) => call[0] as { method: string; resource: string },
    );
    expect(
      requests.filter(
        (req) => req.method === "post" && req.resource === "sources",
      ),
    ).toHaveLength(1);
    expect(requests).toContainEqual(
      expect.objectContaining({ method: "patch", resource: "sources/source1" }),
    );
    expect(
      requests.filter(
        (req) => req.method === "put" && req.resource === "connectors/source1",
      ),
    ).toHaveLength(2);
  });
});

it("clears the typed template explicitly while preserving built-in metadata and false settings", async () => {
  h.metadataConfig = {
    enabled: false,
    metadata: [
      {
        key: "version",
        type: "number",
        custom: { retain: true },
        restrict_values: false,
      },
    ],
    built_in_metadata: [{ key: "filename", custom: "keep" }],
  };
  h.readback.mockImplementation(
    async (
      method: string,
      _id: string,
      _resource: string,
      body: Record<string, unknown>,
    ) => {
      if (method === "put") {
        h.metadataConfig = { ...h.metadataConfig, ...body };
        return true;
      }
      return h.metadataConfig;
    },
  );
  const user = userEvent.setup();
  renderPanel(<KnowledgeMetadataPanel id="kb1" />);
  await user.click(screen.getByRole("button", { name: "删除字段 1" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/元数据配置已保存并回读确认/);
  expect(h.readback).toHaveBeenCalledWith("put", "kb1", "metadata/config", {
    metadata: [],
  });
  expect(h.metadataConfig).toEqual({
    enabled: false,
    metadata: [],
    built_in_metadata: [{ key: "filename", custom: "keep" }],
  });
  expect(h.readback.mock.calls.map(([method]) => method)).toEqual([
    "get",
    "put",
    "get",
  ]);
});
it("keeps metadata drafts after a stale readback and preserves unknown field settings", async () => {
  h.metadataConfig = {
    enabled: false,
    metadata: [
      {
        key: "version",
        type: "number",
        custom: { retain: true },
        restrict_values: false,
      },
    ],
    built_in_metadata: [],
  };
  h.readback.mockResolvedValue(h.metadataConfig);
  const user = userEvent.setup();
  renderPanel(<KnowledgeMetadataPanel id="kb1" />);
  const key = screen.getByRole("textbox", { name: "字段 1 的名称" });
  await user.clear(key);
  await user.type(key, "revision");
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("保存后回读未确认，请重试");
  expect(key).toHaveValue("revision");
  expect(h.readback).toHaveBeenCalledWith(
    "put",
    "kb1",
    "metadata/config",
    expect.objectContaining({
      metadata: [
        {
          key: "revision",
          type: "number",
          custom: { retain: true },
          restrict_values: false,
        },
      ],
    }),
  );
});
