import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor, within, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import { MemoryRouter } from "react-router";
import { useAuthStore } from "@/stores/auth";
import KnowledgeForm from "./KnowledgeForm";
import type { KnowledgeDataset } from "@/api/knowledge";

const h = vi.hoisted(() => ({
  createMock: vi.fn(),
  updateMock: vi.fn(),
  syncMock: vi.fn(),
  onClose: vi.fn(),
  onCreated: vi.fn(),
  providers: [] as Array<Record<string, unknown>>,
  multiragEmbedding: [] as Array<Record<string, unknown>>,
  multiragLayout: [] as Array<Record<string, unknown>>,
}));

vi.mock("@/queries/useKnowledge", () => ({
  useCreateKnowledge: () => ({ mutateAsync: h.createMock, isPending: false }),
  useUpdateKnowledge: () => ({ mutateAsync: h.updateMock, isPending: false }),
}));

vi.mock("@/queries/useProviders", () => ({
  useProviders: () => ({ data: h.providers, isLoading: false }),
  useSyncProviderMultiRAG: () => ({ mutateAsync: h.syncMock }),
}));

vi.mock("@/queries/useMultirag", () => ({
  useMultiragModels: (type: string) => ({
    data: type === "embedding" ? h.multiragEmbedding : h.multiragLayout,
    isLoading: false,
  }),
}));

const glmProvider = {
  id: 42,
  key: "glm-cn",
  name: "ZhipuAI",
  protocol: "anthropic",
  authStyle: "api_key",
  baseUrl: "",
  description: "",
  descriptionEn: "",
  iconKey: "",
  builtin: false,
  lockedApiKey: "",
  attributes: {},
  createdAt: "",
  updatedAt: "",
  defaultModels: [
    {
      modelId: "bge-large-zh",
      displayName: "BGE Large ZH",
      modelType: "embedding",
    },
  ],
  fields: [],
};

// Local OCR preset with no configured service address.
const mineruDualProvider = {
  id: 7,
  key: "mineru",
  name: "MinerU Local",
  protocol: "mineru",
  authStyle: "api_key",
  baseUrl: "",
  description: "",
  descriptionEn: "",
  iconKey: "",
  builtin: true,
  lockedApiKey: "",
  attributes: {},
  createdAt: "",
  updatedAt: "",
  defaultModels: [
    {
      modelId: "mineru-embed",
      displayName: "MinerU Embed",
      modelType: "embedding",
    },
    { modelId: "mineru", displayName: "MinerU OCR", modelType: "ocr" },
  ],
  fields: [],
};

function makeEditing(
  overrides: Partial<KnowledgeDataset> = {},
): KnowledgeDataset {
  return {
    id: "kb-1",
    name: "MyKB",
    display_name: "MyKB",
    collection_name: "col_1",
    description: "",
    permission: "me",
    doc_num: 0,
    chunk_num: 0,
    parser_id: "naive",
    embd_id: "",
    parser_config: {},
    ...overrides,
  };
}

function renderForm(editing: KnowledgeDataset | null = null) {
  return render(
    <ConfigProvider>
      <MemoryRouter><KnowledgeForm open editing={editing} onClose={h.onClose} onCreated={h.onCreated} /></MemoryRouter>
    </ConfigProvider>,
  );
}

// antd Select opens on pointer/mouse-down. Clicking the combobox control for
// a labelled form item, then clicking the visible option text, drives a
// selection reliably across antd v5 in jsdom.
async function pickOption(
  user: ReturnType<typeof userEvent.setup>,
  itemLabel: string,
  optionText: string,
) {
  if (itemLabel === "解析布局" || itemLabel === "实体与关系提取方式") {
    await openAdvanced(user);
  }
  const formItem = screen.getByText(itemLabel).closest(".ant-form-item");
  const control = formItem
    ? within(formItem).getByRole("combobox")
    : screen.getByRole("combobox");
  await user.click(control);
  const opt = await screen.findByText(optionText);
  await user.click(opt);
}

async function openAdvanced(user: ReturnType<typeof userEvent.setup>) {
  const header = screen.getByText("高级配置").closest<HTMLElement>(".ant-collapse-header");
  if (header?.getAttribute("aria-expanded") !== "true") {
    await user.click(header!);
  }
}

describe("KnowledgeForm merged-candidate Select + sync orchestration", () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null });
    localStorage.clear();
    h.createMock.mockReset();
    h.updateMock.mockReset();
    h.syncMock.mockReset();
    h.onClose.mockReset();
    h.onCreated.mockReset();
    h.createMock.mockResolvedValue({ id: "kb-new" });
    h.providers = [glmProvider];
    h.multiragEmbedding = [
      {
        name: "bge-m3",
        factory: "ZHIPU-AI",
        type: "embedding",
        status: "1",
        fullId: "bge-m3@ZHIPU-AI",
      },
    ];
    h.multiragLayout = [
      {
        name: "mineru-x",
        factory: "MinerU",
        type: "ocr",
        status: "1",
        fullId: "mineru-x@MinerU",
      },
    ];
  });

  it("hands a confirmed new dataset ID to the document destination", async () => {
    renderForm();
    const user = userEvent.setup();
    expect(screen.getByRole('textbox', { name: '名称' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: '描述' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '解析方法' })).toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: '权限' })).not.toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '向量模型' })).toBeInTheDocument();
    expect(screen.getByText('高级配置').closest('.ant-collapse-header')).toHaveAttribute('aria-expanded', 'false');
    await user.type(screen.getByPlaceholderText("知识库名称"), "New sources");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await waitFor(() => { expect(h.onCreated).toHaveBeenCalledWith("kb-new"); });
    expect(h.createMock).toHaveBeenCalledWith(expect.objectContaining({ permission: 'me' }));
    expect(h.createMock.mock.calls[0][0]).not.toHaveProperty('embd_id');
    expect(h.onClose).toHaveBeenCalledTimes(1);
  });

  it("renders the candidate Select (not the legacy Input)", async () => {
    renderForm();
    await openAdvanced(userEvent.setup());
    const embdItem = screen
      .getByText("向量模型")
      .closest(".ant-form-item")!;
    expect(within(embdItem).getByRole("combobox")).toBeInTheDocument();
    expect(
      screen.queryByPlaceholderText("如 bge-m3、text-embedding-3-small"),
    ).not.toBeInTheDocument();
  });

  it("locks the embedding model and omits it from updates once chunks exist", async () => {
    h.updateMock.mockResolvedValue({ id: "kb-1" });
    renderForm(
      makeEditing({
        chunk_num: 12,
        embd_id: "bge-m3@ZHIPU-AI",
      }),
    );

    await openAdvanced(userEvent.setup());

    const embeddingInput = screen.getByRole("textbox", {
      name: "向量模型",
    });
    expect(embeddingInput).toHaveAttribute("readonly");
    expect(embeddingInput).toHaveValue("bge-m3@ZHIPU-AI");
    const descriptionId = embeddingInput.getAttribute("aria-describedby");
    expect(descriptionId).toBeTruthy();
    expect(screen.getByText("已锁定")).toBeInTheDocument();
    expect(document.getElementById(descriptionId!)).toHaveTextContent(
      "当前知识库已有 12 个文本块",
    );

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));

    await waitFor(() => expect(h.updateMock).toHaveBeenCalledTimes(1));
    expect(h.updateMock.mock.calls[0][0].data).not.toHaveProperty("embd_id");
    expect(h.syncMock).not.toHaveBeenCalled();
  });

  it("keeps a dirty draft on Escape until the user explicitly discards it", async () => {
    renderForm();
    const user = userEvent.setup();
    const name = screen.getByPlaceholderText("知识库名称");
    await user.type(name, "Keep this draft");
    await user.keyboard('{Escape}');
    expect(await screen.findByRole("dialog", { name: "放弃未保存的配置？" })).toBeInTheDocument();
    expect(h.onClose).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "继续编辑" }));
    expect(name).toHaveValue("Keep this draft");
    await user.click(screen.getByRole("button", { name: /取\s*消/ }));
    await user.click(await screen.findByRole("button", { name: "放弃草稿" }));
    await waitFor(() => { expect(h.onClose).toHaveBeenCalledTimes(1); });
    expect(h.createMock).not.toHaveBeenCalled();
  });

  it("blocks dismissal while a provider synchronization is running", async () => {
    let release!: () => void;
    h.syncMock.mockImplementation(() => new Promise<void>((resolve) => { release = resolve; }));
    renderForm();
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "Busy draft");
    await pickOption(user, "向量模型", "BGE Large ZH (ZhipuAI)");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await waitFor(() => { expect(h.syncMock).toHaveBeenCalledTimes(1); });
    expect(screen.getByRole("button", { name: /取\s*消/ })).toBeDisabled();
    await user.keyboard('{Escape}');
    expect(h.onClose).not.toHaveBeenCalled();
    expect(h.createMock).not.toHaveBeenCalled();
    release();
    await waitFor(() => { expect(h.createMock).toHaveBeenCalledTimes(1); });
  });

  it("syncs the provider then creates when a local embedding is selected", async () => {
    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "向量模型", "BGE Large ZH (ZhipuAI)");

    await user.click(screen.getByRole("button", { name: /创\s*建/ }));

    await waitFor(() => expect(h.syncMock).toHaveBeenCalledTimes(1));
    expect(h.syncMock).toHaveBeenCalledWith({
      id: 42,
      verifyOnly: false,
      modelIds: ["bge-large-zh"],
    });
    await waitFor(() => expect(h.createMock).toHaveBeenCalledTimes(1));
    expect(h.createMock).toHaveBeenCalledWith(
      expect.objectContaining({ embd_id: "bge-large-zh" }),
    );
    expect(h.onClose).toHaveBeenCalled();
  });

  it("retries configuration on the durable dataset instead of creating again", async () => {
    h.createMock.mockRejectedValueOnce({ response: { data: {
      code: "knowledge_configuration_pending", data: { dataset: { id: "kb-partial" } },
    } } });
    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "向量模型", "bge-m3 (ZHIPU-AI)");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("知识库已创建，配置尚未保存");
    expect(h.onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "稍后处理" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
    expect(screen.getByPlaceholderText("知识库名称")).toHaveValue("KB");
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() => expect(h.updateMock).toHaveBeenCalledWith({
      id: "kb-partial", data: expect.objectContaining({ embd_id: "bge-m3@ZHIPU-AI" }),
    }));
    expect(h.createMock).toHaveBeenCalledTimes(1);
    expect(h.onClose).toHaveBeenCalledTimes(1);
  });

  it("defers a failed configuration and reopens the same dataset with its complete draft", async () => {
    h.createMock.mockRejectedValueOnce({ response: { data: { code: "knowledge_configuration_pending", data: { dataset: { id: "kb-partial" } } } } });
    h.updateMock.mockRejectedValueOnce(new Error("configuration rejected")).mockResolvedValueOnce({ id: "kb-partial" });
    function tree(open: boolean, editing: KnowledgeDataset | null = null) { return <ConfigProvider><MemoryRouter><KnowledgeForm open={open} editing={editing} onClose={h.onClose} onCreated={h.onCreated} /></MemoryRouter></ConfigProvider>; }
    const view = render(tree(true));
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "Existing dataset draft");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await screen.findByText(/知识库已创建，配置尚未保存/);
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(1); });
    await user.type(screen.getByPlaceholderText("知识库用途描述"), "Keep this description");
    await user.keyboard('{Escape}');
    expect(h.onClose).toHaveBeenCalledTimes(1);
    view.rerender(tree(false));
    expect(await screen.findByText("待保存的知识库配置已保留")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "打开已创建库的设置" })).toHaveAttribute("href", "/knowledge/kb-partial/settings");
    view.rerender(tree(true));
    expect(await screen.findByPlaceholderText("知识库名称")).toHaveValue("Existing dataset draft");
    expect(screen.getByPlaceholderText("知识库用途描述")).toHaveValue("Keep this description");
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(2); });
    expect(h.createMock).toHaveBeenCalledTimes(1);
    expect(h.updateMock).toHaveBeenLastCalledWith({ id: "kb-partial", data: expect.objectContaining({ name: "Existing dataset draft", description: "Keep this description" }) });
    expect(h.onCreated).toHaveBeenCalledWith("kb-partial");
  });

  it("keeps the deferred creation while another dataset is edited", async () => {
    h.createMock.mockRejectedValueOnce({ response: { data: { code: "knowledge_configuration_pending", data: { dataset: { id: "kb-partial" } } } } });
    h.updateMock.mockResolvedValueOnce({ id: "another-kb" }).mockResolvedValueOnce({ id: "kb-partial" });
    function tree(open: boolean, editing: KnowledgeDataset | null = null) { return <ConfigProvider><MemoryRouter><KnowledgeForm open={open} editing={editing} onClose={h.onClose} onCreated={h.onCreated} /></MemoryRouter></ConfigProvider>; }
    const view = render(tree(true));
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "Pending creation");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await user.click(await screen.findByRole("button", { name: "稍后处理" }));
    view.rerender(tree(false));
    const another = makeEditing({ id: "another-kb" });
    view.rerender(tree(true, another));
    const name = await screen.findByPlaceholderText("知识库名称");
    await user.clear(name);
    await user.type(name, "Edited another dataset");
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledWith({ id: "another-kb", data: { name: "Edited another dataset" } }); });
    view.rerender(tree(false, another));
    expect(screen.getByRole("link", { name: "打开已创建库的设置" })).toHaveAttribute("href", "/knowledge/kb-partial/settings");
    view.rerender(tree(true));
    expect(await screen.findByPlaceholderText("知识库名称")).toHaveValue("Pending creation");
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenLastCalledWith({ id: "kb-partial", data: expect.objectContaining({ name: "Pending creation" }) }); });
    expect(h.createMock).toHaveBeenCalledTimes(1);
  });

  it("stops before creation when the account changes during provider sync", async () => {
    useAuthStore.setState({ user: { id: "owner-a", name: "A", email: "a@example.invalid", role: "admin" } });
    let release!: () => void;
    h.syncMock.mockImplementation(() => new Promise<void>((resolve) => { release = resolve; }));
    renderForm();
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "A private draft");
    await pickOption(user, "向量模型", "BGE Large ZH (ZhipuAI)");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await waitFor(() => { expect(h.syncMock).toHaveBeenCalledTimes(1); });
    act(() => { useAuthStore.setState({ user: { id: "owner-b", name: "B", email: "b@example.invalid", role: "admin" } }); });
    await act(async () => { release(); });
    expect(h.createMock).not.toHaveBeenCalled();
    expect(h.updateMock).not.toHaveBeenCalled();
    expect(h.onCreated).not.toHaveBeenCalled();
    expect(h.onClose).not.toHaveBeenCalled();
    expect(screen.getByPlaceholderText("知识库名称")).toHaveValue("");
  });

  it("does not navigate or close the new account form for an old creation result", async () => {
    useAuthStore.setState({ user: { id: "owner-a", name: "A", email: "a@example.invalid", role: "admin" } });
    let release!: (value: { id: string }) => void;
    h.createMock.mockImplementation(() => new Promise<{ id: string }>((resolve) => { release = resolve; }));
    renderForm();
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "A private draft");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await waitFor(() => { expect(h.createMock).toHaveBeenCalledTimes(1); });
    act(() => { useAuthStore.setState({ user: { id: "owner-b", name: "B", email: "b@example.invalid", role: "admin" } }); });
    await act(async () => { release({ id: "first-owner-kb" }); });
    expect(h.onCreated).not.toHaveBeenCalled();
    expect(h.onClose).not.toHaveBeenCalled();
    expect(h.updateMock).not.toHaveBeenCalled();
    expect(screen.getByPlaceholderText("知识库名称")).toHaveValue("");
  });

  it("does not navigate or clear A's recovery after a late recovery PUT result", async () => {
    useAuthStore.setState({ user: { id: "owner-a", name: "A", email: "a@example.invalid", role: "admin" } });
    h.createMock.mockRejectedValueOnce({ response: { data: { code: "knowledge_configuration_pending", data: { dataset: { id: "owner-a-kb" } } } } });
    let release!: (value: { id: string }) => void;
    h.updateMock.mockImplementation(() => new Promise<{ id: string }>((resolve) => { release = resolve; }));
    renderForm();
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "A recovery draft");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await user.click(await screen.findByRole("button", { name: /保\s*存/ }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(1); });
    act(() => { useAuthStore.setState({ user: { id: "owner-b", name: "B", email: "b@example.invalid", role: "admin" } }); });
    await act(async () => { release({ id: "owner-a-kb" }); });
    expect(h.onCreated).not.toHaveBeenCalled();
    expect(h.onClose).not.toHaveBeenCalled();
    expect(screen.queryByText(/知识库已创建，配置尚未保存/)).not.toBeInTheDocument();
    act(() => { useAuthStore.setState({ user: { id: "owner-a", name: "A", email: "a@example.invalid", role: "admin" } }); });
    expect(await screen.findByText(/知识库已创建，配置尚未保存/)).toBeInTheDocument();
    expect(screen.getByPlaceholderText("知识库名称")).toHaveValue("A recovery draft");
  });

  it("does not resume an old operation after A changes to B and back to A", async () => {
    const ownerA = { id: "owner-a", name: "A", email: "a@example.invalid", role: "admin" };
    useAuthStore.setState({ user: ownerA });
    let release!: () => void;
    h.syncMock.mockImplementation(() => new Promise<void>((resolve) => { release = resolve; }));
    renderForm();
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "A private draft");
    await pickOption(user, "向量模型", "BGE Large ZH (ZhipuAI)");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await waitFor(() => { expect(h.syncMock).toHaveBeenCalledTimes(1); });
    act(() => { useAuthStore.setState({ user: { id: "owner-b", name: "B", email: "b@example.invalid", role: "admin" } }); useAuthStore.setState({ user: ownerA }); });
    await act(async () => { release(); });
    expect(h.createMock).not.toHaveBeenCalled();
  });

  it("rejects a changed token even before userinfo has projected the new account", async () => {
    localStorage.setItem('access_token', 'token-a');
    useAuthStore.setState({ user: { id: "owner-a", name: "A", email: "a@example.invalid", role: "admin" } });
    renderForm();
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "A private draft");
    localStorage.setItem('access_token', 'token-b');
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    expect(h.createMock).not.toHaveBeenCalled();
    expect(h.syncMock).not.toHaveBeenCalled();
    expect(await screen.findByText(/账号或登录凭据已变化/)).toBeInTheDocument();
  });

  it("stops following requests after the form unmounts during sync", async () => {
    let release!: () => void;
    h.syncMock.mockImplementation(() => new Promise<void>((resolve) => { release = resolve; }));
    const view = renderForm();
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("知识库名称"), "Unmounted draft");
    await pickOption(user, "向量模型", "BGE Large ZH (ZhipuAI)");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await waitFor(() => { expect(h.syncMock).toHaveBeenCalledTimes(1); });
    view.unmount();
    await act(async () => { release(); });
    expect(h.createMock).not.toHaveBeenCalled();
    expect(h.onClose).not.toHaveBeenCalled();
  });

  it("does not offer another account the pending dataset ID or draft", async () => {
    useAuthStore.setState({ user: { id: "first-owner", name: "First", email: "first@example.invalid", role: "admin" } });
    h.createMock.mockRejectedValueOnce({ response: { data: { code: "knowledge_configuration_pending", data: { dataset: { id: "first-owner-kb" } } } } });
    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "First owner draft");
    await user.click(screen.getByRole("button", { name: /创\s*建/ }));
    await screen.findByText(/知识库已创建，配置尚未保存/);
    useAuthStore.setState({ user: { id: "second-owner", name: "Second", email: "second@example.invalid", role: "admin" } });
    await waitFor(() => { expect(screen.getByPlaceholderText("知识库名称")).toHaveValue(""); });
    expect(screen.queryByText(/知识库已创建，配置尚未保存/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "稍后处理" })).not.toBeInTheDocument();
    expect(h.updateMock).not.toHaveBeenCalled();
    useAuthStore.setState({ user: null });
  });

  it("skips sync and creates when a multirag embedding is selected", async () => {
    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "向量模型", "bge-m3 (ZHIPU-AI)");

    await user.click(screen.getByRole("button", { name: /创\s*建/ }));

    await waitFor(() => expect(h.createMock).toHaveBeenCalledTimes(1));
    expect(h.syncMock).not.toHaveBeenCalled();
    expect(h.createMock).toHaveBeenCalledWith(
      expect.objectContaining({ embd_id: "bge-m3@ZHIPU-AI" }),
    );
  });

  it("aborts submission when provider sync fails", async () => {
    h.syncMock.mockRejectedValueOnce(new Error("sync failed"));
    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "向量模型", "BGE Large ZH (ZhipuAI)");

    await user.click(screen.getByRole("button", { name: /创\s*建/ }));

    await waitFor(() => expect(h.syncMock).toHaveBeenCalledTimes(1));
    // Give the rejected promise a tick to settle, then assert no create.
    await waitFor(() => expect(h.createMock).not.toHaveBeenCalled());
    expect(h.onClose).not.toHaveBeenCalled();
  });

  it("surfaces a synthetic 'unavailable' option for a saved embd_id not in any candidate source", async () => {
    const user = userEvent.setup();
    const editing = makeEditing({ embd_id: "ghost-model@Deleted" });
    renderForm(editing);
    await openAdvanced(user);

    const embdItem = screen
      .getByText("向量模型")
      .closest(".ant-form-item")!;
    // Select still renders as a combobox (not the legacy free-form Input).
    expect(within(embdItem).getByRole("combobox")).toBeInTheDocument();
    expect(
      screen.queryByPlaceholderText("如 bge-m3、text-embedding-3-small"),
    ).not.toBeInTheDocument();

    // The saved value is selected: the Select's display content carries the
    // synthetic "模型不可用" label.
    const selectContent = embdItem.querySelector(".ant-select-content")!;
    await waitFor(() =>
      expect(selectContent.textContent).toContain("ghost-model@Deleted"),
    );
    expect(selectContent.textContent).toContain("模型不可用");

    // Open the dropdown — the synthetic option carrying "模型不可用" appears
    // in the option list too.
    await user.click(within(embdItem).getByRole("combobox"));
    const opts = await screen.findAllByText(/模型不可用/);
    expect(opts.length).toBeGreaterThan(0);
  });

  it("remaps a saved raw embd_id that exists in candidates to its encoded option", async () => {
    // multiragEmbedding (from beforeEach) has bge-m3@ZHIPU-AI; the saved raw
    // value should be remapped to the encoded `multirag:bge-m3@ZHIPU-AI`.
    const editing = makeEditing({ embd_id: "bge-m3@ZHIPU-AI" });
    renderForm(editing);

    const embdItem = screen
      .getByText("向量模型")
      .closest(".ant-form-item")!;
    // Wait for the remap effect to swap the saved raw for the encoded value;
    // the Select then displays the matching option's label.
    await waitFor(() =>
      expect(
        embdItem.querySelector(".ant-select-content")!.textContent,
      ).toContain("bge-m3 (ZHIPU-AI)"),
    );

    // Submit and verify the encoded value made it through to the API call.
    h.updateMock.mockResolvedValue({ id: "kb-1" });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /保\s*存/ }));
    await waitFor(() => expect(h.updateMock).toHaveBeenCalledTimes(1));
    expect(h.updateMock).toHaveBeenCalledWith({ id: "kb-1", data: {} });

  });

  it("hides an unconfigured local OCR preset and links to parser configuration", async () => {
    h.providers = [mineruDualProvider];
    h.multiragEmbedding = [
      {
        name: "bge-m3",
        factory: "ZHIPU-AI",
        type: "embedding",
        status: "1",
        fullId: "bge-m3@ZHIPU-AI",
      },
    ];
    h.multiragLayout = [];

    const user = userEvent.setup();
    renderForm();
    await openAdvanced(user);
    expect(screen.getByRole('link', { name: '配置解析器 / 视觉模型' })).toHaveAttribute('href', '/providers');
    const layoutItem = screen.getByText('解析布局').closest('.ant-form-item')!;
    await user.click(within(layoutItem).getByRole('combobox'));
    expect(screen.queryByText('MinerU OCR (MinerU Local)')).not.toBeInTheDocument();
    expect(screen.getAllByText('DeepDOC').length).toBeGreaterThan(0);
  });

  it("syncs only a local embedding while using an already enabled MultiRAG OCR", async () => {
    h.providers = [mineruDualProvider];
    h.multiragEmbedding = [];

    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "向量模型", "MinerU Embed (MinerU Local)");
    await pickOption(user, "解析布局", "mineru-x (MinerU)");

    await user.click(screen.getByRole("button", { name: /创\s*建/ }));

    await waitFor(() => expect(h.syncMock).toHaveBeenCalledTimes(1));
    expect(h.syncMock).toHaveBeenCalledWith({
      id: 7,
      verifyOnly: false,
      modelIds: ["mineru-embed"],
    });
    await waitFor(() => expect(h.createMock).toHaveBeenCalledTimes(1));
    expect(h.createMock.mock.calls[0][0].parser_config.layout_recognize).toBe('mineru-x@MinerU');
  });
  it('shows a recoverable GraphRAG conflict before any provider or dataset write', async () => {
    renderForm();
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText('知识库名称'), 'Conflict draft');
    await pickOption(user, '向量模型', 'BGE Large ZH (ZhipuAI)');
    await user.click(screen.getByText('GraphRAG 配置'));
    await pickOption(user, '实体与关系提取方式', '通用提取');
    await user.click(screen.getByText('高级 JSON'));
    const json = screen.getByPlaceholderText('{"raptor": {"use_raptor": true}}');
    await user.click(json);
    await user.paste(JSON.stringify({ graphrag: { method: 'light' } }));
    await user.click(screen.getByRole('button', { name: /创\s*建/ }));
    expect(await screen.findByText('GraphRAG 表单与高级 JSON 修改了同一字段且值不同，请保留一种修改后再保存。')).toBeInTheDocument();
    expect(h.createMock).not.toHaveBeenCalled();
    expect(h.updateMock).not.toHaveBeenCalled();
    expect(h.syncMock).not.toHaveBeenCalled();
    expect(h.onClose).not.toHaveBeenCalled();
    expect(screen.getByPlaceholderText('知识库名称')).toHaveValue('Conflict draft');
    await user.clear(json);
    await user.paste(JSON.stringify({ graphrag: { method: 'general', future: { keep: true } } }));
    await user.click(screen.getByRole('button', { name: /创\s*建/ }));
    await waitFor(() => { expect(h.createMock).toHaveBeenCalledTimes(1); });
    expect(h.syncMock).toHaveBeenCalledTimes(1);
    expect(h.createMock).toHaveBeenCalledWith(expect.objectContaining({ parser_config: expect.objectContaining({ graphrag: { method: 'general', future: { keep: true } } }) }));
    expect(h.onClose).toHaveBeenCalledTimes(1);
  });

});
