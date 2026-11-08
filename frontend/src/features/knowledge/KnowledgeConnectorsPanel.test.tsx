import { beforeEach, expect, it, vi } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import KnowledgeConnectorsPanel from "./KnowledgeConnectorsPanel";
import i18next from "@/i18n";

const h = vi.hoisted(() => ({
  row: {} as Record<string, unknown>,
  mutate: vi.fn(),
  readback: vi.fn(),
  queryError: undefined as Error | undefined,
  logs: [] as Record<string, unknown>[],
}));
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeResource: (_id: string, resource: string) => ({
    data:
      resource === "sources" || resource === "connectors"
        ? [h.row]
        : resource === "sources/s1"
          ? h.row
          : resource === "sources/s1/logs"
            ? { logs: h.logs, total: h.logs.length }
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
  h.readback.mockImplementation(async () => h.row);
  h.queryError = undefined;
  h.logs = [];
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
  "enables supported %s deletion sync while preserving its existing configuration editor",
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
      screen.getByRole("switch", { name: "同步删除来源已移除的文件" }),
    ).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(h.mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        resource: "sources/s1",
        data: expect.objectContaining({
          config: {
            ...(h.row.config as Record<string, unknown>),
            sync_deleted_files: false,
          },
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
  ["mssql", "rdbms_sync_scopes"],
])(
  "leaves %s worker checkpoints under server control during save/readback",
  async (source, stateKey) => {
    h.row = {
      id: "s1",
      name: "Worker state",
      source,
      config: {
        custom: "keep",
        ...(source === "mssql"
          ? {
              host: "fixture",
              database: "kb",
              query: "SELECT id FROM docs",
              credentials: { username: "reader", password: "fixture" },
            }
          : {}),
        [stateKey]: { kb1: { cursor: 1 } },
      },
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
    expect(h.mutate.mock.calls[0][0].data.config).not.toHaveProperty(stateKey);
    expect(h.mutate.mock.calls[0][0].data.config).toMatchObject({
      custom: "keep",
      sync_deleted_files: false,
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
  expect(h.readback).toHaveBeenCalledTimes(2); // initial read + pre-save freshness, no persistence readback
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

const graphCredentials = {
  tenant_id: "fixture-tenant",
  client_id: "fixture-client",
  client_secret: "fixture-secret",
};
const latestSourceFixtures = [
  [
    "rest_api",
    {
      url: "https://fixture.example/items",
      content_fields: ["body"],
      auth_type: "bearer",
      credentials: { token: "fixture-token" },
    },
  ],
  [
    "sharepoint",
    {
      credentials: {
        ...graphCredentials,
        site_url: "https://fixture.sharepoint.com/sites/docs",
      },
    },
  ],
  ["teams", { credentials: graphCredentials, batch_size: 12 }],
  [
    "onedrive",
    { credentials: graphCredentials, drive_ids: [], folder_path: "/Documents" },
  ],
  ["outlook", { credentials: graphCredentials, user_ids: [], folder: "inbox" }],
  [
    "azure_blob",
    {
      auth_mode: "connection_string",
      credentials: {
        connection_string: "fixture",
        container_name: "documents",
      },
    },
  ],
  [
    "salesforce",
    {
      credentials: {
        instance_url: "https://fixture.my.salesforce.com",
        client_id: "client",
        client_secret: "secret",
      },
      objects: [],
    },
  ],
  [
    "mssql",
    {
      host: "fixture",
      port: 1433,
      database: "kb",
      query: "SELECT id, body FROM dbo.docs",
      credentials: { username: "reader", password: "fixture" },
      id_column: "id",
    },
  ],
] as const;

async function settings() {
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  return user;
}
function persist() {
  h.mutate.mockImplementation(
    async (request: { data: Record<string, unknown> }) => {
      h.readback.mockResolvedValue({ ...h.row, ...request.data });
      return true;
    },
  );
}

it.each(latestSourceFixtures)(
  "saves and reads back %s with exact credential/scope shape and unknown extensions",
  async (source, config) => {
    h.row = {
      id: "s1",
      name: "Latest source",
      source,
      config: { ...config, private_extension: { future: true } },
      refresh_freq: 0,
      prune_freq: 0,
      timeout_secs: 30,
    };
    persist();
    const user = await settings();
    expect(screen.getByText("配置字段")).toBeInTheDocument();
    await user.click(screen.getByText("查看配置示例（请替换占位值）"));
    expect(
      screen.getByRole("textbox", { name: "数据源配置与凭证（JSON 对象）" }),
    ).toHaveValue(JSON.stringify(h.row.config, null, 2));
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    const saved = h.mutate.mock.calls[0][0].data.config;
    expect(saved).toMatchObject(config);
    expect(saved.private_extension).toEqual({ future: true });
    expect(h.readback).toHaveBeenCalledTimes(3);
  },
);

it.each([
  ["同步间隔（分钟）", ""],
  ["同步间隔（分钟）", "1.5"],
  ["同步间隔（分钟）", "-1"],
  ["清理间隔（分钟，0 为关闭）", ""],
  ["清理间隔（分钟，0 为关闭）", "2.5"],
  ["任务超时（秒）", ""],
  ["任务超时（秒）", "0"],
  ["任务超时（秒）", "1.5"],
])(
  "rejects invalid schedule %s=%s before any source request",
  async (label, value) => {
    h.row = {
      id: "s1",
      name: "RSS",
      source: "rss",
      config: {
        feed_url: "https://fixture.example/rss",
        sync_deleted_files: true,
      },
    };
    const user = await settings();
    const field = screen.getByRole("spinbutton", { name: label });
    await user.clear(field);
    if (value) await user.type(field, value);
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    expect(
      await screen.findAllByText(/请输入大于等于 \d 的整数/),
    ).not.toHaveLength(0);
    expect(h.mutate).not.toHaveBeenCalled();
    expect(h.readback).toHaveBeenCalledTimes(1); // initial read only; invalid schedule never submits
  },
);

it.each([
  [0, 17],
  [17, 0],
])(
  "persists independent sync=%s and prune=%s schedules",
  async (refresh, prune) => {
    h.row = {
      id: "s1",
      name: "RSS",
      source: "rss",
      config: {
        feed_url: "https://fixture.example/rss",
        sync_deleted_files: true,
      },
      refresh_freq: 60,
      prune_freq: 60,
      timeout_secs: 30,
    };
    persist();
    const user = await settings();
    for (const [label, value] of [
      ["同步间隔（分钟）", refresh],
      ["清理间隔（分钟，0 为关闭）", prune],
    ] as const) {
      const field = screen.getByRole("spinbutton", { name: label });
      await user.clear(field);
      await user.type(field, String(value));
    }
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(h.mutate.mock.calls[0][0].data).toMatchObject({
      refresh_freq: refresh,
      prune_freq: prune,
    });
  },
);

it("disables prune unless deletion is supported and enabled, retaining the stored interval", async () => {
  h.row = {
    id: "s1",
    name: "Moodle",
    source: "moodle",
    config: { sync_deleted_files: false },
    prune_freq: 27,
  };
  const user = await settings();
  expect(
    screen.getByRole("spinbutton", { name: "清理间隔（分钟，0 为关闭）" }),
  ).toBeDisabled();
  await user.click(
    screen.getByRole("switch", { name: "同步删除来源已移除的文件" }),
  );
  expect(
    screen.getByRole("spinbutton", { name: "清理间隔（分钟，0 为关闭）" }),
  ).toBeEnabled();
  expect(
    screen.getByRole("spinbutton", { name: "清理间隔（分钟，0 为关闭）" }),
  ).toHaveValue("27");
});

it("uses the stable pause/resume route and explicit reschedule does not become a readback field", async () => {
  h.row = {
    id: "s1",
    name: "RSS",
    source: "rss",
    status: "2",
    config: { sync_deleted_files: false },
  };
  persist();
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "暂停同步" }));
  await user.click(screen.getByRole("button", { name: "恢复同步" }));
  expect(h.mutate).toHaveBeenNthCalledWith(1, {
    method: "post",
    resource: "sources/s1/resume",
    data: { resume: false },
  });
  expect(h.mutate).toHaveBeenNthCalledWith(2, {
    method: "post",
    resource: "sources/s1/resume",
    data: { resume: true },
  });
  await user.click(screen.getByRole("button", { name: "设置" }));
  h.mutate.mockImplementation(
    async (request: { data: Record<string, unknown> }) => {
      const data = { ...request.data };
      delete data.reschedule;
      h.readback.mockResolvedValue({ ...h.row, ...data });
      return true;
    },
  );
  await user.click(screen.getByRole("switch", { name: "保存后重新调度" }));
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(h.mutate).toHaveBeenLastCalledWith(
    expect.objectContaining({
      method: "patch",
      data: expect.objectContaining({ reschedule: true }),
    }),
  );
});

it("distinguishes prune removals, failed partial sync progress and unknown log task types", async () => {
  h.row = { id: "s1", name: "RSS", source: "rss", config: {} };
  h.logs = [
    {
      id: "prune-log",
      task_type: "prune",
      status: "3",
      docs_removed_from_index: 7,
      new_docs_indexed: 999,
      error_count: 0,
    },
    {
      id: "sync-log",
      task_type: "sync",
      status: "4",
      new_docs_indexed: 2,
      docs_removed_from_index: 888,
      error_count: 1,
    },
    {
      id: "future-log",
      task_type: "future-task",
      status: "1",
      new_docs_indexed: 777,
    },
  ];
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "同步日志" }));
  const prune = within(
    screen.getByRole("button", { name: "prune-log" }).closest("tr")!,
  );
  expect(prune.getByText("删除清理")).toBeInTheDocument();
  expect(prune.getByText("7")).toBeInTheDocument();
  expect(prune.queryByText("999")).not.toBeInTheDocument();
  const sync = within(
    screen.getByRole("button", { name: "sync-log" }).closest("tr")!,
  );
  expect(sync.getByText("失败")).toBeInTheDocument();
  expect(sync.getByText("2")).toBeInTheDocument();
  expect(sync.queryByText("888")).not.toBeInTheDocument();
  expect(screen.getByText("未知（future-task）")).toBeInTheDocument();
  expect(screen.queryByText("777")).not.toBeInTheDocument();
});

it("retains concurrent refreshed credentials and unknown fields without replaying worker state", async () => {
  h.row = {
    id: "s1",
    name: "Gmail",
    source: "gmail",
    config: {
      credentials: { google_tokens: "old-fixture" },
      sync_deleted_files: true,
    },
  };
  h.readback.mockResolvedValueOnce(h.row).mockResolvedValueOnce({
    ...h.row,
    config: {
      ...(h.row.config as Record<string, unknown>),
      credentials: { google_tokens: "fresh-fixture" },
      future_state: { cursor: 12 },
    },
  });
  persist();
  const user = await settings();
  await user.click(
    screen.getByRole("switch", { name: "同步删除来源已移除的文件" }),
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(h.mutate.mock.calls[0][0].data.config).toEqual({
    credentials: { google_tokens: "fresh-fixture" },
    sync_deleted_files: false,
    future_state: { cursor: 12 },
  });
});

it("keeps conflicting configuration drafts open instead of overwriting refreshed fields", async () => {
  h.row = {
    id: "s1",
    name: "Gmail",
    source: "gmail",
    config: { credentials: { google_tokens: "old-fixture" } },
  };
  h.readback.mockResolvedValueOnce(h.row).mockResolvedValue({
    ...h.row,
    config: { credentials: { google_tokens: "fresh-fixture" } },
  });
  const user = await settings();
  const field = screen.getByRole("textbox", {
    name: "数据源配置与凭证（JSON 对象）",
  });
  await user.clear(field);
  await user.paste('{"credentials":{"google_tokens":"edited-fixture"}}');
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText(/来源配置在编辑期间发生变化/);
  expect(field).toHaveValue(
    '{"credentials":{"google_tokens":"edited-fixture"}}',
  );
  expect(h.mutate).not.toHaveBeenCalled();
});

it("does not close when the connector API returns an explicit failed result", async () => {
  h.row = { id: "s1", name: "RSS", source: "rss", config: {} };
  h.mutate.mockResolvedValue(false);
  const user = await settings();
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("保存后回读未确认，请重试");
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  expect(h.readback).toHaveBeenCalledTimes(2);
});

it.each([
  ["rest_api", { url: "https://fixture.example/items" }, "content_fields"],
  ["sharepoint", { credentials: graphCredentials }, "credentials.site_url"],
  [
    "teams",
    { credentials: { tenant_id: "t", client_id: "c" } },
    "credentials.client_secret",
  ],
  ["onedrive", { credentials: graphCredentials }, "drive_ids"],
  ["outlook", { credentials: graphCredentials, user_ids: null }, "user_ids"],
  ["azure_blob", { auth_mode: "unknown", credentials: {} }, "auth_mode"],
  [
    "salesforce",
    { credentials: { instance_url: "https://fixture.my.salesforce.com" } },
    "credentials.client_id",
  ],
  [
    "mssql",
    {
      host: "fixture",
      database: "kb",
      credentials: { username: "reader", password: "fixture" },
    },
    "query",
  ],
])(
  "explains invalid %s configuration without saving or probing a source",
  async (source, config, field) => {
    h.row = { id: "s1", name: "Invalid fixture", source, config };
    const user = await settings();
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await screen.findByText(new RegExp(`配置字段缺失或格式错误：.*${field}`));
    expect(h.mutate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  },
);

it("keeps a created REST connector after link failure and retries its saved identity", async () => {
  h.row = { id: "s1", name: "Existing", source: "rss", config: {} };
  const user = userEvent.setup();
  render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "创建数据源连接器" }));
  const dialog = within(screen.getByRole("dialog"));
  await user.type(
    dialog.getByRole("textbox", { name: "名称" }),
    "Controlled REST",
  );
  await user.click(dialog.getByRole("combobox", { name: "来源" }));
  await user.type(dialog.getByRole("combobox", { name: "来源" }), "rest_api");
  await user.click(
    screen.getByText("rest_api", {
      selector: ".ant-select-item-option-content",
    }),
  );
  expect(
    screen.getByText(/必填 url 和至少一个 content_fields/),
  ).toBeInTheDocument();
  const config = dialog.getByRole("textbox", {
    name: "数据源配置与凭证（JSON 对象）",
  });
  await user.clear(config);
  await user.paste(
    '{"url":"https://fixture.example/items","content_fields":["body"],"credentials":{"extension":"keep"}}',
  );
  let linked = false;
  h.mutate.mockImplementation(
    async (request: {
      method: string;
      data: Record<string, unknown>;
      resource: string;
    }) => {
      if (request.method === "post") {
        h.readback.mockResolvedValue({ id: "created", ...request.data });
        return "created";
      }
      if (request.method === "patch") {
        h.readback.mockResolvedValue({ id: "created", ...request.data });
        return true;
      }
      if (!linked) {
        linked = true;
        throw new Error("Controlled link failure");
      }
      return true;
    },
  );
  await user.click(dialog.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("Controlled link failure");
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  await user.click(dialog.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(
    h.mutate.mock.calls.filter(([request]) => request.method === "post"),
  ).toHaveLength(1);
  expect(h.mutate).toHaveBeenLastCalledWith({
    method: "put",
    resource: "connectors/created",
    data: { auto_parse: true },
  });
});

it("keeps Zendesk ticket deletion disabled even when JSON requests it", async () => {
  h.row = {
    id: "s1",
    name: "Tickets",
    source: "zendesk",
    config: { zendesk_content_type: "tickets", sync_deleted_files: true },
  };
  const user = await settings();
  expect(
    screen.queryByRole("switch", { name: "同步删除来源已移除的文件" }),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole("spinbutton", { name: "清理间隔（分钟，0 为关闭）" }),
  ).toBeDisabled();
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  expect(
    await screen.findAllByText(
      "此来源或配置不支持删除同步，请勿启用 sync_deleted_files。",
    ),
  ).not.toHaveLength(0);
  expect(h.mutate).not.toHaveBeenCalled();
});

it("keeps the original draft baseline when the detail query refreshes in the background", async () => {
  h.row = {
    id: "s1",
    name: "Gmail",
    source: "gmail",
    config: {
      credentials: { google_tokens: "old-fixture" },
      sync_deleted_files: true,
    },
  };
  persist();
  const user = userEvent.setup();
  const view = render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  h.row = {
    ...h.row,
    config: {
      credentials: { google_tokens: "fresh-fixture" },
      sync_deleted_files: true,
      future_extension: "fresh",
    },
  };
  view.rerender(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(
    screen.getByRole("switch", { name: "同步删除来源已移除的文件" }),
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(h.mutate.mock.calls[0][0].data.config).toEqual({
    credentials: { google_tokens: "fresh-fixture" },
    sync_deleted_files: false,
    future_extension: "fresh",
  });
});

it("reopens from fresh persisted detail after a successful PATCH whose readback failed, despite stale query cache", async () => {
  h.row = {
    id: "s1",
    name: "Cached RSS",
    source: "rss",
    refresh_freq: 0,
    prune_freq: 11,
    timeout_secs: 41,
    config: {
      feed_url: "https://fixture.example/rss",
      sync_deleted_files: true,
      unknown_preserved: { a: 2 },
    },
  };
  let server = structuredClone(h.row);
  let readbackFailure = false;
  h.readback.mockImplementation(async () => {
    if (readbackFailure) {
      readbackFailure = false;
      throw new Error("Controlled persistence readback failed");
    }
    return structuredClone(server);
  });
  h.mutate.mockImplementation(
    async (request: { data: Record<string, unknown> }) => {
      server = { ...server, ...request.data };
      readbackFailure = true;
      return true;
    },
  );
  const user = await settings();
  for (const [label, value] of [
    ["清理间隔（分钟，0 为关闭）", "13"],
    ["任务超时（秒）", "43"],
  ]) {
    await user.clear(screen.getByRole("spinbutton", { name: label }));
    await user.type(screen.getByRole("spinbutton", { name: label }), value);
  }
  const config = screen.getByRole("textbox", {
    name: "数据源配置与凭证（JSON 对象）",
  });
  await user.clear(config);
  await user.paste(
    JSON.stringify({
      ...(server.config as Record<string, unknown>),
      unknown_preserved: { a: 3 },
    }),
  );
  await user.click(screen.getByRole("button", { name: /保\s*存/ }));
  await screen.findByText("Controlled persistence readback failed");
  expect(h.mutate).toHaveBeenCalledTimes(1);
  expect(
    screen.getByRole("spinbutton", { name: "清理间隔（分钟，0 为关闭）" }),
  ).toHaveValue("13");
  await user.click(screen.getByRole("button", { name: /取\s*消/ }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  let finishRead!: (value: Record<string, unknown>) => void;
  h.readback.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishRead = resolve;
      }),
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("textbox", { name: "数据源配置与凭证（JSON 对象）" }),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: /保\s*存/ })).toBeDisabled();
  await act(async () => {
    finishRead(structuredClone(server));
  });
  expect(
    screen.getByRole("spinbutton", { name: "清理间隔（分钟，0 为关闭）" }),
  ).toHaveValue("13");
  expect(
    screen.getByRole("spinbutton", { name: "任务超时（秒）" }),
  ).toHaveValue("43");
  expect(
    JSON.parse(
      (
        screen.getByRole("textbox", {
          name: "数据源配置与凭证（JSON 对象）",
        }) as HTMLTextAreaElement
      ).value,
    ).unknown_preserved,
  ).toEqual({ a: 3 });
  expect(h.row.prune_freq).toBe(11); // React Query's stale cache was never used to initialize the reopened form.
  expect(h.row.timeout_secs).toBe(41);
  expect(h.mutate).toHaveBeenCalledTimes(1);
});

it("ignores a previous opening's late GET after the fresh reopened form has been edited", async () => {
  h.row = {
    id: "s1",
    name: "Cached",
    source: "rss",
    config: { sync_deleted_files: true },
    prune_freq: 11,
    timeout_secs: 41,
  };
  let finishOldRead!: (value: Record<string, unknown>) => void;
  h.readback.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishOldRead = resolve;
      }),
  );
  const user = userEvent.setup();
  const view = render(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  await user.click(screen.getByRole("button", { name: "设置" }));
  expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: /取\s*消/ }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  h.readback.mockResolvedValue({
    ...h.row,
    prune_freq: 13,
    timeout_secs: 43,
    config: { sync_deleted_files: true, unknown: "fresh" },
  });
  await user.click(screen.getByRole("button", { name: "设置" }));
  const prune = await screen.findByRole("spinbutton", {
    name: "清理间隔（分钟，0 为关闭）",
  });
  await user.clear(prune);
  await user.type(prune, "17");
  await act(async () => {
    finishOldRead(h.row);
  });
  h.row = {
    ...h.row,
    prune_freq: 1,
    timeout_secs: 1,
    config: { unknown: "late cache refresh" },
  };
  view.rerender(
    <ConfigProvider>
      <KnowledgeConnectorsPanel id="kb1" />
    </ConfigProvider>,
  );
  expect(prune).toHaveValue("17");
  expect(
    screen.getByRole("spinbutton", { name: "任务超时（秒）" }),
  ).toHaveValue("43");
  expect(
    JSON.parse(
      (
        screen.getByRole("textbox", {
          name: "数据源配置与凭证（JSON 对象）",
        }) as HTMLTextAreaElement
      ).value,
    ).unknown,
  ).toBe("fresh");
});

it("requires a successful fresh read before editing and permits retry without showing stale cached fields", async () => {
  h.row = {
    id: "s1",
    name: "Cached",
    source: "rss",
    config: {},
    prune_freq: 11,
    timeout_secs: 41,
  };
  h.readback.mockRejectedValueOnce(new Error("Controlled fresh open failure"));
  const user = await settings();
  await screen.findByText("Controlled fresh open failure");
  expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: /保\s*存/ })).toBeDisabled();
  h.readback.mockResolvedValue({ ...h.row, prune_freq: 13, timeout_secs: 43 });
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: /刷\s*新/ }),
  );
  expect(
    await screen.findByRole("spinbutton", { name: "任务超时（秒）" }),
  ).toHaveValue("43");
  expect(h.mutate).not.toHaveBeenCalled();
});
