import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import { MemoryRouter, Route, Routes } from "react-router";
import { antdTheme } from "@/lib/antd-theme";
import KnowledgeSettingsPage from "./KnowledgeSettingsPage";
import KnowledgeDetailPage from "./KnowledgeDetailPage";
import { setAuthRole } from "@/test/auth-store-mock";

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock("@/stores/auth", async () => {
  const { createAuthStoreMock, getAuthUser } = await import("@/test/auth-store-mock");
  const mock = createAuthStoreMock();
  Object.assign(mock.useAuthStore, { getState: () => ({ user: { ...getAuthUser(), id: h.userId } }), subscribe: () => () => undefined });
  return mock;
});

const h = vi.hoisted(() => ({
  dataset: {} as Record<string, unknown> | undefined,
  error: false,
  loading: false,
  userId: '1',
  origin: { id: '1', token: 'token-a', role: 'admin' },
  updateMock: vi.fn(),
  refetchMock: vi.fn(),
  syncMock: vi.fn(),
  providers: [] as Record<string, unknown>[],
  multiragEmbedding: [] as Record<string, unknown>[],
  multiragVision: [] as Record<string, unknown>[],
}));

vi.mock("@/queries/useKnowledge", () => ({
  useKnowledgeDetail: () => ({
    data: h.dataset,
    origin: h.origin,
    isLoading: h.loading,
    isError: h.error,
    refetch: h.refetchMock,
  }),
  useUpdateKnowledge: () => ({
    mutateAsync: h.updateMock,
    isPending: false,
  }),
}));

vi.mock("@/queries/useProviders", () => ({
  useProviders: () => ({ data: h.providers, isLoading: false }),
  useSyncProviderMultiRAG: () => ({
    mutateAsync: h.syncMock,
    isPending: false,
  }),
}));

vi.mock("@/queries/useMultirag", () => ({
  useMultiragModels: (type: string) => ({
    data: type === 'image2text' ? h.multiragVision : h.multiragEmbedding,
    isLoading: false,
  }),
}));

const baseDataset = {
  id: "kb-1",
  name: "Product docs",
  display_name: "Product docs",
  collection_name: "product_docs",
  description: "",
  permission: "me",
  doc_num: 1,
  chunk_num: 0,
  parser_id: "naive",
  embd_id: "bge-m3@ZHIPU-AI",
  parser_config: {},
};

function pageTree() {
  return (
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={["/knowledge/kb-1/settings"]}>
        <Routes>
          <Route
            path="/knowledge/:id/settings"
            element={<KnowledgeSettingsPage />}
          />
        </Routes>
      </MemoryRouter>
    </ConfigProvider>
  );
}
function renderPage() { return render(pageTree()); }

describe("KnowledgeSettingsPage embedding model guard", () => {
  beforeEach(() => {
    setAuthRole("admin");
    h.userId = '1';
    h.origin = { id: '1', token: 'token-a', role: 'admin' };
    localStorage.setItem('access_token', 'token-a');
    h.dataset = { ...baseDataset };
    h.error = false;
    h.loading = false;
    h.updateMock.mockReset();
    h.updateMock.mockResolvedValue({ id: "kb-1" });
    h.refetchMock.mockReset();
    h.refetchMock.mockResolvedValue({});
    h.syncMock.mockReset();
    h.syncMock.mockResolvedValue({});
    h.providers = [];
    h.multiragVision = [];
    h.multiragEmbedding = [
      {
        name: "bge-m3",
        factory: "ZHIPU-AI",
        type: "embedding",
        status: "1",
        fullId: "bge-m3@ZHIPU-AI",
      },
    ];
  });

  it("blocks editing and saving after an initial failed read", async () => {
    h.error = true;
    h.dataset = undefined;
    renderPage();
    expect(screen.getByText("设置加载失败")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "保存设置" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "名称" })).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: /重\s*试/ }));
    expect(h.refetchMock).toHaveBeenCalledTimes(1);
    expect(h.updateMock).not.toHaveBeenCalled();
  });

  it("retains the nested settings draft through a failed refresh and retry", async () => {
    function tree() { return <ConfigProvider theme={antdTheme}><MemoryRouter initialEntries={["/knowledge/kb-1/settings"]}><Routes><Route path="/knowledge/:id" element={<KnowledgeDetailPage />}><Route path="settings" element={<KnowledgeSettingsPage />} /></Route></Routes></MemoryRouter></ConfigProvider>; }
    const view = render(tree());
    const user = userEvent.setup();
    const name = screen.getByRole("textbox", { name: "名称" });
    await user.clear(name);
    await user.type(name, "Unsaved draft");
    h.error = true;
    view.rerender(tree());
    expect(screen.getByText("知识库状态未更新")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "名称" })).toBe(name);
    expect(name).toHaveValue("Unsaved draft");
    expect(name).toBeDisabled();
    expect(screen.getByRole("button", { name: "保存设置" })).toBeDisabled();
    expect(h.updateMock).not.toHaveBeenCalled();
    h.error = false;
    h.dataset = { ...baseDataset, name: "New server value" };
    view.rerender(tree());
    expect(screen.getByRole("textbox", { name: "名称" })).toBe(name);
    expect(name).toHaveValue("Unsaved draft");
    expect(screen.getByRole("button", { name: "保存设置" })).toBeEnabled();
  });

  it("does not expose default configuration when no dataset was returned", () => {
    h.dataset = undefined;
    renderPage();
    expect(screen.getByText("设置加载失败")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "保存设置" })).not.toBeInTheDocument();
  });

  it("keeps an unsaved draft when query data refreshes", async () => {
    const user = userEvent.setup();
    const view = renderPage();
    const input = screen.getByRole("textbox", { name: "名称" });
    await user.clear(input);
    await user.type(input, "Draft name");
    h.dataset = { ...baseDataset, name: "Server name" };
    view.rerender(<ConfigProvider theme={antdTheme}><MemoryRouter initialEntries={["/knowledge/kb-1/settings"]}><Routes><Route path="/knowledge/:id/settings" element={<KnowledgeSettingsPage />} /></Routes></MemoryRouter></ConfigProvider>);
    expect(input).toHaveValue("Draft name");
  });

  it("does not resubmit unedited fields from an old draft after a background refresh", async () => {
    const user = userEvent.setup();
    const view = renderPage();
    await user.type(screen.getByRole("textbox", { name: "描述" }), "My draft");
    h.dataset = { ...baseDataset, name: "Server renamed" };
    view.rerender(<ConfigProvider theme={antdTheme}><MemoryRouter initialEntries={["/knowledge/kb-1/settings"]}><Routes><Route path="/knowledge/:id/settings" element={<KnowledgeSettingsPage />} /></Routes></MemoryRouter></ConfigProvider>);
    await user.click(screen.getByRole("button", { name: "保存设置" }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(1); });
    expect(h.updateMock).toHaveBeenCalledWith({ id: "kb-1", data: { description: "My draft" } });
  });

  it('uses confirmed save readback to preserve a newly unavailable model before the dataset query refreshes', async () => {
    const first = { ...baseDataset, parser_config: { layout_recognize: 'vision-a@Anthropic', vendor: { retained: false } } };
    const saved = { ...first, parser_config: { ...first.parser_config, layout_recognize: 'vision-b@Anthropic' } };
    h.dataset = first;
    h.multiragVision = ['vision-a', 'vision-b'].map(name => ({ name, factory: 'Anthropic', type: 'image2text', status: '1', fullId: `${name}@Anthropic` }));
    h.updateMock.mockResolvedValue(saved);
    const view = renderPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole('combobox', { name: '解析布局' }));
    await user.click(await screen.findByText('vision-b (Anthropic)'));
    await user.click(screen.getByRole('button', { name: '保存设置' }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(1); });
    await waitFor(() => expect(screen.getByRole('button', { name: '保存设置' })).toBeEnabled());
    h.multiragVision = [];
    view.rerender(pageTree());
    expect(await screen.findByText(/vision-b@Anthropic.*当前未提供/)).toBeInTheDocument();
    await user.type(screen.getByRole('textbox', { name: '描述' }), 'After confirmed save');
    await user.click(screen.getByRole('button', { name: '保存设置' }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(2); });
    expect(h.updateMock.mock.calls[1][0]).toEqual({ id: 'kb-1', data: { description: 'After confirmed save' } });
    expect(h.syncMock).not.toHaveBeenCalled();
  });

  it("renders a searchable candidate selector while the knowledge base is empty", async () => {
    renderPage();

    const item = screen
      .getByText("向量模型")
      .closest<HTMLElement>(".ant-form-item");
    expect(item).not.toBeNull();
    await waitFor(() =>
      expect(within(item!).getByRole("combobox")).toBeInTheDocument(),
    );
    expect(
      screen.queryByPlaceholderText("如 bge-m3、text-embedding-3-small"),
    ).not.toBeInTheDocument();
  });

  it("renders the current model read-only and omits it from locked updates", async () => {
    h.dataset = { ...baseDataset, chunk_num: 8 };
    renderPage();

    const input = await screen.findByRole("textbox", {
      name: "向量模型",
    });
    expect(input).toHaveAttribute("readonly");
    expect(input).toHaveValue("bge-m3@ZHIPU-AI");
    const descriptionId = input.getAttribute("aria-describedby");
    expect(descriptionId).toBeTruthy();
    expect(document.getElementById(descriptionId!)).toHaveTextContent(
      "当前知识库已有 8 个文本块",
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "保存设置" }));

    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(1); });
    expect(h.updateMock.mock.calls[0][0].data).not.toHaveProperty("embd_id");
    expect(h.syncMock).not.toHaveBeenCalled();
  });

  it("refreshes and shows an inline error if chunks appear before save", async () => {
    h.updateMock.mockRejectedValueOnce(
      new Error(
        "When chunk_num (2) > 0, embedding_model must remain bge-m3@ZHIPU-AI",
      ),
    );
    renderPage();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "保存设置" }));

    expect(
      await screen.findByText(
        "知识库已生成文本块，向量模型已锁定。页面已刷新，请重试。",
      ),
    ).toBeInTheDocument();
    expect(h.refetchMock).toHaveBeenCalledTimes(1);
  });

  it("member: hides save button and renders the form read-only", () => {
    setAuthRole("member");
    renderPage();

    // 表单数据仍可见（只读）
    const item = screen
      .getByText("向量模型")
      .closest<HTMLElement>(".ant-form-item");
    expect(item).not.toBeNull();
    // 保存设置按钮隐藏，表单控件整体置为只读（antd Form disabled 经 context 下发）
    expect(screen.queryByRole("button", { name: "保存设置" })).not.toBeInTheDocument();
    expect(within(item!).getByRole("combobox")).toBeDisabled();
  });
});

describe('structured GraphRAG settings', () => {
  beforeEach(() => {
    setAuthRole('admin');
    h.userId = '1';
    h.origin = { id: '1', token: 'token-a', role: 'admin' };
    localStorage.setItem('access_token', 'token-a');
    h.error = false;
    h.loading = false;
    h.updateMock.mockReset();
    h.providers = [];
    h.multiragEmbedding = [];
    h.syncMock.mockReset().mockResolvedValue({});
  });
it('saves explicit GraphRAG disabling without leaving DataFlow or replacing extension fields', async () => {
  const graph = { use_graphrag: true, method: 'general', entity_types: ['person'], community: true, resolution: false, future: { keep: true } };
  h.dataset = { ...baseDataset, pipeline_id: 'existing-flow', parser_config: { graphrag: graph, custom: { keep: true } } };
  h.updateMock.mockResolvedValue(h.dataset);
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByText('GraphRAG 配置'));
  expect(screen.getByText(/手动运行 GraphRAG 索引会重新启用/)).toBeInTheDocument();
  await user.click(screen.getByRole('switch', { name: '启用 GraphRAG' }));
  await user.click(screen.getByRole('switch', { name: '生成社区报告' }));
  await user.click(screen.getByRole('button', { name: '保存设置' }));
  await waitFor(() => { expect(h.updateMock).toHaveBeenCalledWith({ id: 'kb-1', data: { parser_config: { graphrag: { use_graphrag: false, community: false } } } }); });
});

it('retains unsupported GraphRAG in advanced JSON when another setting changes', async () => {
  h.dataset = { ...baseDataset, pipeline_id: 'existing-flow', parser_config: { graphrag: { use_graphrag: true, method: 'future' } } };
  h.updateMock.mockResolvedValue(h.dataset);
  renderPage();
  const user = userEvent.setup();
  await user.click(screen.getByText('GraphRAG 配置'));
  expect(screen.getByText(/现有 GraphRAG 配置包含不兼容/)).toBeInTheDocument();
  expect(screen.queryByRole('switch', { name: '启用 GraphRAG' })).not.toBeInTheDocument();
  await user.type(screen.getByRole('textbox', { name: '描述' }), 'New description');
  await user.click(screen.getByRole('button', { name: '保存设置' }));
  await waitFor(() => { expect(h.updateMock).toHaveBeenCalledWith({ id: 'kb-1', data: { description: 'New description' } }); });
});

it.each(['token-only', 'userinfo', 'unmount'])('stops GraphRAG PUT after provider sync when %s interrupts the initiating identity', async (phase) => {
  h.dataset = { ...baseDataset, chunk_num: 0, parser_config: { graphrag: { community: true } } };
  h.providers = [{ id: 42, key: 'local', name: 'Local', protocol: 'anthropic', defaultModels: [{ modelId: 'local-embed', displayName: 'Local embedding', modelType: 'embedding' }] }];
  let finish: () => void = () => { throw new Error('Sync not started'); };
  h.syncMock.mockImplementation(() => new Promise((resolve) => { finish = () => { resolve({}); }; }));
  const user = userEvent.setup();
  const view = renderPage();
  const combo = screen.getByRole('combobox', { name: '向量模型' });
  await user.click(combo);
  await user.click(screen.getByText('Local embedding (Local)'));
  await user.click(screen.getByText('GraphRAG 配置'));
  await user.click(screen.getByRole('switch', { name: '生成社区报告' }));
  await user.click(screen.getByRole('button', { name: '保存设置' }));
  await waitFor(() => { expect(h.syncMock).toHaveBeenCalledTimes(1); });
  if (phase === 'token-only') localStorage.setItem('access_token', 'token-b');
  else if (phase === 'userinfo') h.userId = '2';
  else view.unmount();
  await act(async () => { finish(); });
  expect(h.updateMock).not.toHaveBeenCalled();
  if (phase === 'token-only') expect(await screen.findByText(/账号或登录凭据已变化/)).toBeInTheDocument();
});

it('blocks a save from old displayed settings before userinfo catches up with a new token', async () => {
  h.dataset = { ...baseDataset, parser_config: { graphrag: { community: true } } };
  renderPage();
  const user = userEvent.setup();
  await user.click(screen.getByText('GraphRAG 配置'));
  await user.click(screen.getByRole('switch', { name: '生成社区报告' }));
  localStorage.setItem('access_token', 'token-b');
  await user.click(screen.getByRole('button', { name: '保存设置' }));
  expect(h.updateMock).not.toHaveBeenCalled();
  expect(h.syncMock).not.toHaveBeenCalled();
  expect(await screen.findByText(/账号或登录凭据已变化/)).toBeInTheDocument();
});

it('rebases structured fields and advanced JSON after readback so a second description edit cannot overwrite server extensions', async () => {
  h.dataset = { ...baseDataset, pipeline_id: 'flow', parser_config: { custom: { version: 'old' }, graphrag: { community: true, future: { version: 'old', keep: true } } } };
  const persisted = { ...baseDataset, pipeline_id: 'flow', parser_config: { custom: { version: 'server-new' }, graphrag: { community: false, future: { version: 'server-new', keep: true } } } };
  h.updateMock.mockResolvedValue(persisted);
  renderPage();
  const user = userEvent.setup();
  await user.click(screen.getByText('GraphRAG 配置'));
  await user.click(screen.getByRole('switch', { name: '生成社区报告' }));
  await user.click(screen.getByRole('button', { name: '保存设置' }));
  await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(1); });
  await waitFor(() => { expect(screen.getByRole('button', { name: '保存设置' })).not.toHaveClass('ant-btn-loading'); });
  await user.click(screen.getByText('高级 JSON'));
  expect(screen.getByPlaceholderText('{"raptor": {"use_raptor": true}}')).toHaveValue(JSON.stringify({ custom: { version: 'server-new' }, graphrag: { community: false, future: { version: 'server-new', keep: true } } }, null, 2));
  await user.type(screen.getByRole('textbox', { name: '描述' }), 'Only description');
  await user.click(screen.getByRole('button', { name: '保存设置' }));
  await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(2); });
  expect(h.updateMock.mock.calls[1][0]).toEqual({ id: 'kb-1', data: { description: 'Only description' } });
});

});
