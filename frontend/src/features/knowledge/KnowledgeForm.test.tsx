import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import KnowledgeForm from "./KnowledgeForm";
import type { KnowledgeDataset } from "@/api/knowledge";

const h = vi.hoisted(() => ({
  createMock: vi.fn(),
  updateMock: vi.fn(),
  syncMock: vi.fn(),
  onClose: vi.fn(),
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

// Provider whose key maps to a MultiRAG factory and which carries BOTH an
// embedding and an ocr model — used to verify that a single provider is
// synced only once when it supplies both fields (dedup).
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
      <KnowledgeForm open editing={editing} onClose={h.onClose} />
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
  const formItem = screen.getByText(itemLabel).closest(".ant-form-item");
  const control = formItem
    ? within(formItem).getByRole("combobox")
    : screen.getByRole("combobox");
  await user.click(control);
  const opt = await screen.findByText(optionText);
  await user.click(opt);
}

describe("KnowledgeForm merged-candidate Select + sync orchestration", () => {
  beforeEach(() => {
    h.createMock.mockReset();
    h.updateMock.mockReset();
    h.syncMock.mockReset();
    h.onClose.mockReset();
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

  it("renders the candidate Select (not the legacy Input)", () => {
    renderForm();
    const embdItem = screen
      .getByText("Embedding 模型")
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

    const embeddingInput = screen.getByRole("textbox", {
      name: "Embedding 模型",
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

  it("syncs the provider then creates when a local embedding is selected", async () => {
    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "Embedding 模型", "BGE Large ZH (ZhipuAI)");

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

  it("skips sync and creates when a multirag embedding is selected", async () => {
    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "Embedding 模型", "bge-m3 (ZHIPU-AI)");

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
    await pickOption(user, "Embedding 模型", "BGE Large ZH (ZhipuAI)");

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

    const embdItem = screen
      .getByText("Embedding 模型")
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
      .getByText("Embedding 模型")
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
    expect(h.updateMock).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "kb-1",
        data: expect.objectContaining({ embd_id: "bge-m3@ZHIPU-AI" }),
      }),
    );
  });

  it("syncs only the layout provider when embd_id is multirag and layout is local", async () => {
    // Provide a local MinerU OCR candidate without conflicting multirag ocr.
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
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "Embedding 模型", "bge-m3 (ZHIPU-AI)");
    await pickOption(user, "解析布局", "MinerU OCR (MinerU Local)");

    await user.click(screen.getByRole("button", { name: /创\s*建/ }));

    await waitFor(() => expect(h.syncMock).toHaveBeenCalledTimes(1));
    expect(h.syncMock).toHaveBeenCalledWith({
      id: 7,
      verifyOnly: false,
      modelIds: ["mineru"],
    });
    await waitFor(() => expect(h.createMock).toHaveBeenCalledTimes(1));
  });

  it("dedupes sync targets when one provider supplies both embd and layout", async () => {
    // mineruDualProvider (id=7) supplies both an embedding and an ocr model;
    // selecting both should result in exactly ONE sync call for provider 7.
    h.providers = [mineruDualProvider];
    h.multiragEmbedding = [];
    h.multiragLayout = [];

    const user = userEvent.setup();
    renderForm();
    await user.type(screen.getByPlaceholderText("知识库名称"), "KB");
    await pickOption(user, "Embedding 模型", "MinerU Embed (MinerU Local)");
    await pickOption(user, "解析布局", "MinerU OCR (MinerU Local)");

    await user.click(screen.getByRole("button", { name: /创\s*建/ }));

    await waitFor(() => expect(h.syncMock).toHaveBeenCalledTimes(1));
    expect(h.syncMock).toHaveBeenCalledWith({
      id: 7,
      verifyOnly: false,
      modelIds: ["mineru-embed", "mineru"],
    });
    await waitFor(() => expect(h.createMock).toHaveBeenCalledTimes(1));
  });
});
