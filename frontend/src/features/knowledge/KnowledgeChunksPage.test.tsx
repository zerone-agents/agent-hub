import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router";
import { ConfigProvider } from "antd";
import { antdTheme } from "@/lib/antd-theme";
import KnowledgeChunksPage from "./KnowledgeChunksPage";
import { setAuthRole } from "@/test/auth-store-mock";

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock("@/stores/auth", async () => (await import("@/test/auth-store-mock")).createAuthStoreMock());

const h = vi.hoisted(() => ({
  chunks: [] as Record<string, unknown>[],
  total: 0,
  refetchMock: vi.fn(),
  createMock: vi.fn(),
  updateMock: vi.fn(),
  deleteMock: vi.fn(),
  switchMock: vi.fn(),
  fetchImageMock: vi.fn(),
}));

vi.mock("@/queries/useKnowledge", () => ({
  useChunks: () => ({
    data: {
      chunks: h.chunks,
      total: h.total,
      document: {
        id: "d1",
        name: "guide.pdf",
        parser_id: "naive",
        source_type: "upload",
        meta_fields: [{ key: "author" }],
      },
    },
    isLoading: false,
    isFetching: false,
    refetch: h.refetchMock,
  }),
  useCreateChunk: () => ({ mutateAsync: h.createMock, isPending: false }),
  useUpdateChunk: () => ({ mutateAsync: h.updateMock, isPending: false }),
  useDeleteChunks: () => ({ mutate: h.deleteMock }),
  useSwitchChunks: () => ({ mutate: h.switchMock }),
}));

vi.mock("@/api/knowledge", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/knowledge")>();
  return {
    ...actual,
    knowledgeApi: {
      ...actual.knowledgeApi,
      images: {
        ...actual.knowledgeApi.images,
        fetch: h.fetchImageMock,
      },
    },
  };
});

const sampleChunks = [
  {
    id: "c1",
    content: "original chunk content",
    document_id: "d1",
    important_keywords: ["k1"],
    questions: ["q1"],
    available: true,
    positions: [1, 2],
    doc_type: "text",
    tag_kwd: ["manual"],
    tag_feas: {},
  },
  {
    id: "c2",
    content: "<em>image chunk</em>",
    document_id: "d1",
    important_keywords: [],
    questions: [],
    available: false,
    positions: [],
    doc_type: "image",
    tag_kwd: [],
    tag_feas: {},
    image_id: "img1",
  },
];

function renderPage() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={["/knowledge/kb1/documents/d1/chunks"]}>
        <Routes>
          <Route
            path="/knowledge/:id/documents/:documentId/chunks"
            element={<KnowledgeChunksPage />}
          />
        </Routes>
      </MemoryRouter>
    </ConfigProvider>,
  );
}

describe("KnowledgeChunksPage", () => {
  beforeEach(() => {
    setAuthRole("admin");
    h.chunks = sampleChunks;
    h.total = sampleChunks.length;
    h.refetchMock.mockReset();
    h.createMock.mockReset();
    h.createMock.mockResolvedValue({});
    h.updateMock.mockReset();
    h.updateMock.mockResolvedValue({});
    h.deleteMock.mockReset();
    h.switchMock.mockReset();
    h.fetchImageMock.mockReset();
    h.fetchImageMock.mockResolvedValue(
      new Blob(["image"], { type: "image/png" }),
    );
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: vi.fn(() => "blob:chunk-image"),
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: vi.fn(),
    });
  });

  it("renders the chunk workbench cards and fetches images through the gateway", async () => {
    renderPage();

    expect(screen.getByText("original chunk content")).toBeInTheDocument();
    expect(screen.getByText("文档信息")).toBeInTheDocument();
    expect(screen.getByText("ID c1")).toBeInTheDocument();
    expect(h.fetchImageMock).toHaveBeenCalledWith(
      "kb1",
      "img1",
      expect.any(AbortSignal),
    );
    expect(
      await screen.findByRole("img", { name: "chunk image" }),
    ).toHaveAttribute("src", "blob:chunk-image");
  }, 15000);

  it("shows an inline fallback when a chunk image request fails", async () => {
    h.fetchImageMock.mockRejectedValueOnce(new Error("image not found"));
    renderPage();

    expect(await screen.findByText("图片加载失败")).toBeInTheDocument();
    expect(screen.getByText("image not found")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "复制 ID" })).toBeInTheDocument();
  }, 15000);

  it("saves an edited chunk from the drawer", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getAllByRole("button", { name: /编辑/ })[0]);
    const textarea = await screen.findByPlaceholderText("切片文本内容");
    await user.clear(textarea);
    await user.type(textarea, "updated content");
    await user.click(screen.getByRole("button", { name: "保存切片" }));

    await waitFor(() => { expect(h.updateMock).toHaveBeenCalled(); });
    expect(h.updateMock).toHaveBeenCalledWith(
      expect.objectContaining({
        chunkId: "c1",
        input: expect.objectContaining({ content: "updated content" }),
      }),
    );
  }, 15000);

  it("supports selection and bulk switch", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("checkbox", { name: "选择本页" }));
    await user.click(screen.getByRole("button", { name: "批量停用切片" }));

    expect(h.switchMock).toHaveBeenCalledWith({
      chunkIds: ["c1", "c2"],
      available: false,
    });
  });

  it("creates an image chunk with base64 payload", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("button", { name: "新增切片" }));
    const textarea = await screen.findByPlaceholderText("切片文本内容");
    await user.type(textarea, "new image chunk");

    const fileInputs = Array.from(
      document.querySelectorAll('input[type="file"]'),
    ) as HTMLInputElement[];
    const fileInput = fileInputs[fileInputs.length - 1];
    await user.upload(
      fileInput,
      new File(["image"], "a.png", { type: "image/png" }),
    );

    expect(
      await screen.findByRole("img", { name: "preview image" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "创建切片" }));

    await waitFor(() => { expect(h.createMock).toHaveBeenCalled(); });
    expect(h.createMock).toHaveBeenCalledWith(
      expect.objectContaining({
        content: "new image chunk",
        image_base64: "aW1hZ2U=",
      }),
    );
  }, 15000);

  it("member: hides editor/switch/delete/bulk but keeps data and copy ID", () => {
    setAuthRole("member");
    renderPage();

    // 数据仍可见（只读）
    expect(screen.getByText("original chunk content")).toBeInTheDocument();
    expect(screen.getByText("文档信息")).toBeInTheDocument();
    // 写操作按钮隐藏：新增切片/编辑/删除/启用 Switch
    expect(screen.queryByRole("button", { name: "新增切片" })).not.toBeInTheDocument();
    expect(screen.queryAllByRole("button", { name: /编辑/ })).toHaveLength(0);
    expect(screen.queryAllByRole("button", { name: /删除/ })).toHaveLength(0);
    expect(document.querySelector(".ant-switch")).toBeNull();
    // 批量勾选与批量栏不存在
    expect(screen.queryByRole("checkbox", { name: "选择本页" })).not.toBeInTheDocument();
    expect(screen.queryByText(/已选择/)).not.toBeInTheDocument();
  });
});
