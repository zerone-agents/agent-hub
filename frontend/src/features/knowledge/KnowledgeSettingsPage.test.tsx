import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigProvider } from "antd";
import { MemoryRouter, Route, Routes } from "react-router";
import { antdTheme } from "@/lib/antd-theme";
import KnowledgeSettingsPage from "./KnowledgeSettingsPage";

const h = vi.hoisted(() => ({
  dataset: {},
  updateMock: vi.fn(),
  refetchMock: vi.fn(),
  syncMock: vi.fn(),
  providers: [] as Record<string, unknown>[],
  multiragEmbedding: [] as Record<string, unknown>[],
}));

vi.mock("@/queries/useKnowledge", () => ({
  useKnowledgeDetail: () => ({
    data: h.dataset,
    isLoading: false,
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
  useMultiragModels: () => ({
    data: h.multiragEmbedding,
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

function renderPage() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={["/knowledge/kb-1/settings"]}>
        <Routes>
          <Route
            path="/knowledge/:id/settings"
            element={<KnowledgeSettingsPage />}
          />
        </Routes>
      </MemoryRouter>
    </ConfigProvider>,
  );
}

describe("KnowledgeSettingsPage embedding model guard", () => {
  beforeEach(() => {
    h.dataset = { ...baseDataset };
    h.updateMock.mockReset();
    h.updateMock.mockResolvedValue({ id: "kb-1" });
    h.refetchMock.mockReset();
    h.refetchMock.mockResolvedValue({});
    h.syncMock.mockReset();
    h.syncMock.mockResolvedValue({});
    h.providers = [];
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

  it("renders a searchable candidate selector while the knowledge base is empty", async () => {
    renderPage();

    const item = screen
      .getByText("Embedding 模型")
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
      name: "Embedding 模型",
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
});
