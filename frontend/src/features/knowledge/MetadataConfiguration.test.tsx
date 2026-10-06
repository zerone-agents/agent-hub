import { beforeEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import KnowledgeMetadataPanel from "./KnowledgeMetadataPanel";
import { setAuthRole } from "@/test/auth-store-mock";
const h = vi.hoisted(() => ({
  config: {} as Record<string, unknown>,
  error: null as unknown,
  request: vi.fn(),
  refetch: vi.fn(),
}));
vi.mock("@/stores/auth", async () =>
  (await import("@/test/auth-store-mock")).createAuthStoreMock(),
);
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeResource: (_id: string, resource: string) => ({
    data: resource === "metadata/config" ? h.config : { summary: {} },
    isLoading: false,
    isFetching: false,
    refetch: h.refetch,
    error: resource === "metadata/config" ? h.error : null,
  }),
  useKnowledgeMetadataInventory: () => ({
    data: { keys: [], flattened: {} },
    refetch: h.refetch,
  }),
}));
vi.mock("@/api/knowledgeManagement", async (original) => ({
  ...(await original<typeof import("@/api/knowledgeManagement")>()),
  knowledgeRequest: h.request,
}));
beforeEach(() => {
  setAuthRole("admin");
  h.config = {};
  h.error = null;
  h.request.mockReset();
  h.refetch.mockReset();
});
it("blocks the legacy enabled-only request when the template snapshot is missing", async () => {
  const user = userEvent.setup();
  h.request.mockResolvedValue({ enabled: false });
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("switch", { name: "自动提取元数据" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(
    "元数据配置不完整，请重新加载后保存，以保留已有模板。",
  );
  expect(h.request).not.toHaveBeenCalled();
});
it("disables saving stale configuration after a loading error", () => {
  h.error = new Error("permission denied");
  h.config = { enabled: true, metadata: [] };
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  expect(screen.getByText("permission denied")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /保\s*存/ })).toBeDisabled();
});
it("toggles enabled with the committed HEAD partial envelope and preserves a newer untouched template", async () => {
  h.config = { enabled: true, metadata: [{ key: "old", custom: false }] };
  const latest = {
    enabled: true,
    metadata: [{ key: "new", custom: { retain: 0 } }],
    built_in_metadata: [{ key: "filename" }],
  };
  let stored = latest;
  h.request.mockImplementation(async (method, _id, _resource, body) => {
    if (method === "put") {
      stored = { ...stored, ...body };
      return true;
    }
    return stored;
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("switch", { name: "自动提取元数据" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/元数据配置已保存并回读确认/);
  expect(h.request).toHaveBeenCalledWith("put", "kb1", "metadata/config", {
    enabled: false,
  });
  expect(stored.metadata).toEqual(latest.metadata);
  expect(screen.getByRole("textbox", { name: "字段 1 的名称" })).toHaveValue(
    "new",
  );
});
it("refuses to overwrite a template changed since the draft was loaded", async () => {
  h.config = { enabled: true, metadata: [{ key: "old" }] };
  h.request.mockResolvedValue({
    enabled: false,
    metadata: [{ key: "concurrent" }],
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  const key = screen.getByRole("textbox", { name: "字段 1 的名称" });
  await user.clear(key);
  await user.type(key, "draft");
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(
    "编辑期间已存模板发生变化，草稿已保留；请重新加载并核对后再保存。",
  );
  expect(key).toHaveValue("draft");
  expect(h.request.mock.calls.some(([method]) => method === "put")).toBe(false);
});
it("saves an explicit template clear without resending enable or built-in fields", async () => {
  h.config = {
    enabled: false,
    metadata: [{ key: "old", custom: 0 }],
    built_in_metadata: [{ key: "filename", custom: false }],
  };
  let stored = h.config;
  h.request.mockImplementation(async (method, _id, _resource, body) => {
    if (method === "put") {
      stored = { ...stored, ...body };
      return true;
    }
    return stored;
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "删除字段 1" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/元数据配置已保存并回读确认/);
  expect(h.request).toHaveBeenCalledWith("put", "kb1", "metadata/config", {
    metadata: [],
  });
  expect(stored.enabled).toBe(false);
  expect(stored.built_in_metadata).toEqual(h.config.built_in_metadata);
});
it("does not claim an enable save succeeded if readback loses the untouched template", async () => {
  h.config = { enabled: true, metadata: [{ key: "old" }] };
  h.request.mockImplementation(async (method) =>
    method === "get" && h.request.mock.calls.some(([verb]) => verb === "put")
      ? { enabled: false, metadata: [] }
      : h.config,
  );
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("switch", { name: "自动提取元数据" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("保存后回读未确认，请重试");
  expect(
    screen.queryByText(/元数据配置已保存并回读确认/),
  ).not.toBeInTheDocument();
});
it("saves only the edited template and preserves an enable change made elsewhere", async () => {
  h.config = { enabled: true, metadata: [{ key: "old" }] };
  let stored = { enabled: false, metadata: [{ key: "old" }] };
  h.request.mockImplementation(async (method, _id, _resource, body) => {
    if (method === "put") {
      stored = { ...stored, ...body };
      return true;
    }
    return stored;
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  const key = screen.getByRole("textbox", { name: "字段 1 的名称" });
  await user.clear(key);
  await user.type(key, "new");
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/元数据配置已保存并回读确认/);
  expect(h.request).toHaveBeenCalledWith("put", "kb1", "metadata/config", {
    metadata: [{ key: "new" }],
  });
  expect(stored.enabled).toBe(false);
});

it("retains an edited template when background configuration refresh fails", async () => {
  h.config = { enabled: true, metadata: [{ key: "old" }] };
  const user = userEvent.setup();
  const view = render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  const key = screen.getByRole("textbox", { name: "字段 1 的名称" });
  await user.clear(key);
  await user.type(key, "draft");
  h.error = new Error("refresh failed");
  view.rerender(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  expect(key).toHaveValue("draft");
  expect(screen.getByRole("button", { name: /保\s*存/ })).toBeDisabled();
  expect(h.request).not.toHaveBeenCalled();
});
it("allows enabled-only saves without validating or rewriting historical definitions", async () => {
  h.config = {
    enabled: true,
    metadata: [{ key: "legacy_boolean", type: "boolean" }],
    built_in_metadata: [{ key: "legacy_builtin", type: "boolean" }],
  };
  let stored = h.config;
  h.request.mockImplementation(async (method, _id, _resource, body) => {
    if (method === "put") stored = { ...stored, ...body };
    return stored;
  });
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("switch", { name: "自动提取元数据" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/元数据配置已保存并回读确认/);
  expect(h.request).toHaveBeenCalledWith("put", "kb1", "metadata/config", {
    enabled: false,
  });
  expect(stored.metadata).toEqual(h.config.metadata);
  expect(stored.built_in_metadata).toEqual(h.config.built_in_metadata);
});
it("still validates an actual edit to a historical definition before writing", async () => {
  h.config = {
    enabled: true,
    metadata: [{ key: "legacy_boolean", type: "boolean" }],
  };
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeMetadataPanel id="kb1" />
    </ConfigProvider>,
  );
  const key = screen.getByRole("textbox", { name: "字段 1 的名称" });
  await user.type(key, "_edited");
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/请检查字段名称、类型与可选值/);
  expect(h.request).not.toHaveBeenCalled();
});
