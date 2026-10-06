import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import KnowledgeConnectorsPanel from "./KnowledgeConnectorsPanel";

const h = vi.hoisted(() => ({
  row: {} as Record<string, unknown>,
  mutate: vi.fn(),
  readback: vi.fn(),
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
    name: "IMAP",
    source: "imap",
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
