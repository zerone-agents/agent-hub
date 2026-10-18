import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import KnowledgeConnectorsPanel from "./KnowledgeConnectorsPanel";
import i18next from "@/i18n";

const h = vi.hoisted(() => ({
  row: {} as Record<string, unknown>,
  mutate: vi.fn(),
  readback: vi.fn(),
  queryError: undefined as Error | undefined,
}));
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeResource: (_id: string, resource: string) => ({
    data:
      resource === "sources" || resource === "connectors"
        ? [h.row]
        : resource === "sources/s1"
          ? h.row
          : undefined,
    refetch: vi.fn(),
    isLoading: false,
    isFetching: false,
    error: resource === "sources/s1" ? h.queryError : undefined,
  }),
  useKnowledgeAction: () => ({
    mutateAsync: h.mutate,
    mutate: h.mutate,
    isPending: false,
  }),
}));
vi.mock("@/api/knowledgeManagement", async (original) => ({
  ...(await original<typeof import("@/api/knowledgeManagement")>()),
  knowledgeRequest: h.readback,
}));
beforeEach(() => {
  h.mutate.mockReset();
  h.readback.mockReset();
  h.queryError = undefined;
});
it("turns off Gmail deletion sync without dropping credentials or unknown source fields and checks readback", async () => {
  h.row = {
    id: "s1",
    name: "Mail",
    source: "gmail",
    auto_parse: "1",
    config: {
      credentials: { future_token: "controlled-fixture" },
      custom: { key: 2 },
      sync_deleted_files: true,
    },
    refresh_freq: 60,
    prune_freq: 0,
    timeout_secs: 3600,
  };
  h.mutate.mockImplementation(
    async (request: { data: Record<string, unknown> }) => {
      h.readback.mockResolvedValue({ ...h.row, ...request.data });
      return true;
    },
  );
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  const toggle = screen.getByRole("switch", {
    name: "同步删除来源已移除的文件",
  });
  expect(toggle).toBeChecked();
  await user.click(toggle);
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(h.mutate).toHaveBeenCalledWith(
    expect.objectContaining({
      method: "patch",
      resource: "sources/s1",
      data: expect.objectContaining({
        config: {
          credentials: { future_token: "controlled-fixture" },
          custom: { key: 2 },
          sync_deleted_files: false,
        },
      }),
    }),
  );
  expect(h.readback).toHaveBeenCalledWith("get", "kb1", "sources/s1");
});
it("keeps the toggle draft when deletion sync readback does not match", async () => {
  h.row = {
    id: "s1",
    name: "RSS",
    source: "rss",
    config: { sync_deleted_files: true },
  };
  h.mutate.mockResolvedValue(true);
  h.readback.mockResolvedValue(h.row);
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  await user.click(
    screen.getByRole("switch", { name: "同步删除来源已移除的文件" }),
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("保存后回读未确认，请重试");
  expect(
    screen.getByRole("switch", { name: "同步删除来源已移除的文件" }),
  ).not.toBeChecked();
  expect(screen.getByRole("dialog")).toBeInTheDocument();
});
it("does not offer a deletion toggle for an unsupported source and retains its configuration", async () => {
  h.row = {
    id: "s1",
    name: "Slack",
    source: "slack",
    config: { custom: "keep", sync_deleted_files: true },
  };
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  expect(
    screen.queryByRole("switch", { name: "同步删除来源已移除的文件" }),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole("textbox", { name: "数据源配置与凭证（JSON 对象）" }),
  ).toHaveValue(JSON.stringify(h.row.config, null, 2));
});

it.each(["dingtalk_ai_table", "imap", "mysql", "postgresql", "moodle"])(
  "keeps new %s deletion capabilities gated while preserving its existing configuration editor",
  async (source) => {
    h.row = {
      id: "s1",
      name: "Controlled source",
      source,
      config: { custom: "keep", credentials: { token: "fixture" } },
    };
    h.mutate.mockImplementation(
      async (request: { data: Record<string, unknown> }) => {
        h.readback.mockResolvedValue({ ...h.row, ...request.data });
        return true;
      },
    );
    const user = userEvent.setup();
    render(
      <ConfigProvider>
        <KnowledgeConnectorsPanel id="kb1" />
      </ConfigProvider>,
    );
    await user.click(screen.getByRole("button", { name: "设置" }));
    expect(
      screen.queryByRole("switch", { name: "同步删除来源已移除的文件" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(h.mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        resource: "sources/s1",
        data: expect.objectContaining({
          config: h.row.config,
        }),
      }),
    );
    expect(h.readback).toHaveBeenCalledWith("get", "kb1", "sources/s1");
  },
);

it.each([
  ["imap", "imap_sync_scopes"],
  ["mysql", "rdbms_sync_scopes"],
  ["postgresql", "rdbms_sync_scopes"],
])(
  "leaves %s worker checkpoints under server control during save/readback",
  async (source, stateKey) => {
    h.row = {
      id: "s1",
      name: "Worker state",
      source,
      config: { custom: "keep", [stateKey]: { kb1: { cursor: 1 } } },
    };
    h.mutate.mockImplementation(
      async (request: { data: { config: Record<string, unknown> } }) => {
        h.readback.mockResolvedValue({
          ...h.row,
          ...request.data,
          config: {
            ...request.data.config,
            [stateKey]: { kb1: { cursor: 2 } },
          },
        });
        return true;
      },
    );
    const user = userEvent.setup();
    render(
      <ConfigProvider>
        <KnowledgeConnectorsPanel id="kb1" />
      </ConfigProvider>,
    );
    await user.click(screen.getByRole("button", { name: "设置" }));
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(h.mutate.mock.calls[0][0].data.config).toEqual({
      custom: "keep",
    });
  },
);

it("does not show a stale enabled status when config refresh fails", async () => {
  h.row = {
    id: "s1",
    name: "Failed source",
    source: "s3",
    config: { sync_deleted_files: true },
  };
  h.queryError = new Error("Controlled configuration read failed");
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  expect(screen.getByText("配置状态未确认")).toBeInTheDocument();
  expect(screen.queryByText("启用")).not.toBeInTheDocument();
});

it("keeps settings open when persistence fails and does not claim readback", async () => {
  h.row = { id: "s1", name: "Failed source", source: "s3", config: {} };
  h.mutate.mockRejectedValue(new Error("Controlled save failed"));
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("Controlled save failed");
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  expect(h.readback).not.toHaveBeenCalled();
});

it.each(["s3", "r2", "google_cloud_storage", "oci_storage"])(
  "requires deletion sync off before changing %s bucket scope",
  async (source) => {
    h.row = {
      id: "s1",
      name: "Blob source",
      source,
      config: { bucket_name: "owned", prefix: "old", sync_deleted_files: true },
    };
    h.mutate.mockImplementation(
      async (request: { data: Record<string, unknown> }) => {
        h.readback.mockResolvedValue({ ...h.row, ...request.data });
        return true;
      },
    );
    const user = userEvent.setup();
    render(
      <ConfigProvider>
        <KnowledgeConnectorsPanel id="kb1" />
      </ConfigProvider>,
    );
    await user.click(screen.getByRole("button", { name: "设置" }));
    const config = screen.getByRole("textbox", {
      name: "数据源配置与凭证（JSON 对象）",
    });
    await user.clear(config);
    await user.paste(
      JSON.stringify({
        bucket_name: "owned",
        prefix: "narrower",
        sync_deleted_files: true,
      }),
    );
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await screen.findByText(
      i18next.t("knowledge.manage.connectorBlobScopeChanged"),
    );
    expect(h.mutate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.click(
      screen.getByRole("switch", { name: "同步删除来源已移除的文件" }),
    );
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(h.mutate.mock.calls[0][0].data.config).toEqual({
      bucket_name: "owned",
      prefix: "narrower",
      sync_deleted_files: false,
    });
  },
);

it("allows credential rotation, unknown fields and equivalent blob prefixes without disabling deletion sync", async () => {
  h.row = {
    id: "s1",
    name: "Blob source",
    source: "s3",
    config: {
      bucket_name: "owned",
      prefix: "docs",
      credentials: { aws_secret_access_key: "old-fixture" },
      sync_deleted_files: true,
    },
  };
  h.mutate.mockImplementation(
    async (request: { data: Record<string, unknown> }) => {
      h.readback.mockResolvedValue({ ...h.row, ...request.data });
      return true;
    },
  );
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  const config = screen.getByRole("textbox", {
    name: "数据源配置与凭证（JSON 对象）",
  });
  const desired = {
    bucket_name: "owned",
    prefix: "docs/",
    custom: "preserved",
    credentials: { aws_secret_access_key: "new-fixture" },
    sync_deleted_files: true,
  };
  await user.clear(config);
  await user.paste(JSON.stringify(desired));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(h.mutate.mock.calls[0][0].data.config).toEqual(desired);
});
