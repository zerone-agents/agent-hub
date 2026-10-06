import { describe, it, expect, beforeEach, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router";
import { ConfigProvider } from "antd";
import { antdTheme } from "@/lib/antd-theme";
import KnowledgeChunksPage from "./KnowledgeChunksPage";
import { setAuthRole } from "@/test/auth-store-mock";

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock("@/stores/auth", async () =>
  (await import("@/test/auth-store-mock")).createAuthStoreMock(),
);

const h = vi.hoisted(() => ({
  chunks: [] as Record<string, unknown>[],
  total: 0,
  refetchMock: vi.fn(),
  createMock: vi.fn(),
  updateMock: vi.fn(),
  deleteMock: vi.fn(),
  switchMock: vi.fn(),
  fetchImageMock: vi.fn(),
  downloadMock: vi.fn(),
  readbackMock: vi.fn(),
  target: null as Record<string, unknown> | null,
}));

vi.mock("@/queries/useKnowledge", () => ({
  useChunks: (_id: string, _documentId: string, params: { id?: string }) => ({
    data: {
      chunks: params.id ? (h.target ? [h.target] : []) : h.chunks,
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
  useDeleteChunks: () => ({ mutate: h.deleteMock, mutateAsync: h.deleteMock }),
  useSwitchChunks: () => ({ mutate: h.switchMock, mutateAsync: h.switchMock }),
}));

vi.mock("@/api/knowledge", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/knowledge")>();
  return {
    ...actual,
    knowledgeApi: {
      ...actual.knowledgeApi,
      chunks: { ...actual.knowledgeApi.chunks, list: h.readbackMock },
      images: {
        ...actual.knowledgeApi.images,
        fetch: h.fetchImageMock,
      },
    },
  };
});

vi.mock("@/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/client")>();
  return {
    ...actual,
    default: {
      ...actual.default,
      get: async (...args: unknown[]) => {
        const response = (await h.downloadMock(...args)) as { data: Blob };
        return {
          data: new ReadableStream<Uint8Array>(
            {
              async pull(controller) {
                controller.enqueue(
                  new Uint8Array(await response.data.arrayBuffer()),
                );
                controller.close();
              },
            },
            { highWaterMark: 0 },
          ),
          headers: { "content-type": response.data.type },
        };
      },
    },
  };
});

vi.mock("./KnowledgePdfPreview", () => ({
  default: ({ page }: { page: number }) => (
    <div role="img" aria-label="PDF original" data-page={page} />
  ),
}));

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

function renderPage(path = "/knowledge/kb1/documents/d1/chunks") {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={[path]}>
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
    h.target = null;
    h.readbackMock.mockReset();
    h.readbackMock.mockResolvedValue({
      chunks: sampleChunks,
      total: sampleChunks.length,
    });
    h.downloadMock.mockReset();
    h.downloadMock.mockResolvedValue({
      data: new Blob(["%PDF-1.7"], { type: "application/pdf" }),
    });
    Object.defineProperty(Blob.prototype, "arrayBuffer", {
      configurable: true,
      value: function (this: Blob) {
        return new Promise((resolve, reject) => {
          const reader = new FileReader();
          reader.onload = () => {
            resolve(reader.result);
          };
          reader.onerror = reject;
          reader.readAsArrayBuffer(this);
        });
      },
    });
    h.total = sampleChunks.length;
    h.refetchMock.mockReset();
    h.createMock.mockReset();
    h.createMock.mockImplementation(async (input) => ({
      id: "new",
      document_id: "d1",
      ...input,
      available: true,
      positions: [],
      tag_feas: input.tag_feas ?? {},
      image_id: input.image_base64 ? "img-new" : undefined,
    }));
    h.updateMock.mockReset();
    h.updateMock.mockImplementation(async ({ chunkId, input }) => ({
      ...sampleChunks.find((chunk) => chunk.id === chunkId),
      ...input,
      document_id: "d1",
      tag_feas: input.tag_feas ?? {},
      image_id: input.image_base64 ? "img1" : undefined,
    }));
    h.deleteMock.mockReset();
    h.switchMock.mockReset();
    h.switchMock.mockResolvedValue({});
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

    await waitFor(() => {
      expect(h.updateMock).toHaveBeenCalled();
    });
    expect(h.updateMock).toHaveBeenCalledWith(
      expect.objectContaining({
        chunkId: "c1",
        input: expect.objectContaining({ content: "updated content" }),
      }),
    );
  }, 15000);

  it("supports arrow keys when the view-switch group itself has focus", () => {
    renderPage();
    const group = screen.getByLabelText("原文与切片核对");
    fireEvent.keyDown(group, { key: "ArrowLeft" });
    expect(group.querySelectorAll('input[type="radio"]')[0]).toBeChecked();
    fireEvent.keyDown(group, { key: "ArrowRight" });
    expect(group.querySelectorAll('input[type="radio"]')[1]).toBeChecked();
  });

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

    await waitFor(() => {
      expect(h.createMock).toHaveBeenCalled();
    });
    expect(h.createMock).toHaveBeenCalledWith(
      expect.objectContaining({
        content: "new image chunk",
        image_base64: "aW1hZ2U=",
      }),
    );
  }, 15000);

  it("updates an existing image with explicit replacement and verifies its protected bytes", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getAllByRole("button", { name: /编辑/ })[1]);
    const fileInput = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    await user.upload(
      fileInput,
      new File(["image"], "replacement.png", { type: "image/png" }),
    );
    expect(
      await screen.findByRole("img", { name: "preview image" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "保存切片" }));
    await waitFor(() => {
      expect(h.updateMock).toHaveBeenCalledWith(
        {
          chunkId: "c2",
          input: {
            image_base64: "aW1hZ2U=",
            image_update_mode: "replace",
          },
        },
      );
    });
    await waitFor(() => {
      expect(h.refetchMock).toHaveBeenCalled();
    });
    expect(
      h.fetchImageMock.mock.calls.filter((call) => call[1] === "img1").length,
    ).toBeGreaterThan(2);
  }, 15000);

  it("keeps the draft when persisted readback disagrees with the submitted content", async () => {
    const user = userEvent.setup();
    h.updateMock.mockResolvedValueOnce(sampleChunks[0]);
    renderPage();
    await user.click(screen.getAllByRole("button", { name: /编辑/ })[0]);
    const textarea = await screen.findByPlaceholderText("切片文本内容");
    await user.clear(textarea);
    await user.type(textarea, "unconfirmed content");
    await user.click(screen.getByRole("button", { name: "保存切片" }));
    expect(
      await screen.findByText(/写入已返回，但回读内容未确认/),
    ).toBeInTheDocument();
    expect(textarea).toHaveValue("unconfirmed content");
    expect(h.refetchMock).not.toHaveBeenCalled();
  }, 15000);

  it("does not confirm an image update if protected bytes still contain the old image", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getAllByRole("button", { name: /编辑/ })[1]);
    await user.upload(
      document.querySelector('input[type="file"]') as HTMLInputElement,
      new File(["different"], "new.png", { type: "image/png" }),
    );
    await screen.findByRole("img", { name: "preview image" });
    await user.click(screen.getByRole("button", { name: "保存切片" }));
    expect(
      await screen.findByText(/写入已返回，但回读内容未确认/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: "preview image" }),
    ).toBeInTheDocument();
    expect(h.refetchMock).not.toHaveBeenCalled();
  }, 15000);

  it("rechecks an uncertain save without repeating the write", async () => {
    const user = userEvent.setup();
    h.updateMock.mockResolvedValueOnce(sampleChunks[0]);
    h.readbackMock.mockResolvedValueOnce({
      chunks: [{ ...sampleChunks[0], content: "confirmed content" }],
      total: 1,
    });
    renderPage();
    await user.click(screen.getAllByRole("button", { name: /编辑/ })[0]);
    const textarea = await screen.findByPlaceholderText("切片文本内容");
    await user.clear(textarea);
    await user.type(textarea, "confirmed content");
    await user.click(screen.getByRole("button", { name: "保存切片" }));
    expect(
      await screen.findByText(/写入已返回，但回读内容未确认/),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "保存切片" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "重新核对" }));
    await waitFor(() => {
      expect(h.refetchMock).toHaveBeenCalled();
    });
    expect(h.readbackMock).toHaveBeenCalledWith("kb1", "d1", {
      id: "c1",
      page_size: 1,
    });
    expect(h.updateMock).toHaveBeenCalledTimes(1);
  }, 15000);

  it("recovers an uncertain replacement only after both text and protected image bytes agree", async () => {
    const user = userEvent.setup();
    h.updateMock.mockRejectedValueOnce(
      new Error("Image write outcome unknown"),
    );
    h.readbackMock.mockResolvedValue({
      chunks: [{ ...sampleChunks[1], content: "replacement caption" }],
      total: 1,
    });
    renderPage();
    await user.click(screen.getAllByRole("button", { name: /编辑/ })[1]);
    const textarea = await screen.findByPlaceholderText("切片文本内容");
    await user.clear(textarea);
    await user.type(textarea, "replacement caption");
    await user.upload(
      document.querySelector('input[type="file"]') as HTMLInputElement,
      new File(["new image"], "replacement.png", { type: "image/png" }),
    );
    await screen.findByRole("img", { name: "preview image" });
    await user.click(screen.getByRole("button", { name: "保存切片" }));

    expect(
      await screen.findByText(/Image write outcome unknown/),
    ).toBeInTheDocument();
    expect(textarea).toHaveValue("replacement caption");
    expect(
      screen.getByRole("img", { name: "preview image" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "保存切片" })).toBeDisabled();
    expect(h.updateMock.mock.calls[0][0]).toEqual({
      chunkId: "c2",
      input: {
        content: "replacement caption",
        image_base64: "bmV3IGltYWdl",
        image_update_mode: "replace",
      },
    });

    // The index can already match while storage still returns the old bytes.
    await user.click(screen.getByRole("button", { name: "重新核对" }));
    expect(
      await screen.findByText(/写入已返回，但回读内容未确认/),
    ).toBeInTheDocument();
    expect(h.refetchMock).not.toHaveBeenCalled();
    expect(h.updateMock).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("img", { name: "preview image" }),
    ).toBeInTheDocument();

    h.fetchImageMock.mockResolvedValue(
      new Blob(["new image"], { type: "image/png" }),
    );
    await user.click(screen.getByRole("button", { name: "重新核对" }));
    await waitFor(() => {
      expect(h.refetchMock).toHaveBeenCalled();
    });
    expect(h.readbackMock).toHaveBeenCalledTimes(2);
    expect(h.readbackMock).toHaveBeenLastCalledWith("kb1", "d1", {
      id: "c2",
      page_size: 1,
    });
    expect(h.updateMock).toHaveBeenCalledTimes(1);
  }, 30000);

  it("keeps an unsaved draft when the editor is closed and reopened", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getAllByRole("button", { name: /编辑/ })[0]);
    const textarea = await screen.findByPlaceholderText("切片文本内容");
    await user.clear(textarea);
    await user.type(textarea, "draft preserved");
    await user.click(screen.getByRole("button", { name: /取.*消/ }));
    await user.click(screen.getAllByRole("button", { name: /编辑/ })[0]);
    expect(await screen.findByPlaceholderText("切片文本内容")).toHaveValue(
      "draft preserved",
    );
    expect(h.updateMock).not.toHaveBeenCalled();
  }, 15000);

  it("preserves bulk selection after a failed switch", async () => {
    const user = userEvent.setup();
    h.switchMock.mockRejectedValueOnce(new Error("switch failed"));
    renderPage();
    await user.click(screen.getByRole("checkbox", { name: "选择本页" }));
    await user.click(screen.getByRole("button", { name: "批量停用切片" }));
    expect(await screen.findByText("switch failed")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "选择本页" })).toBeChecked();
  });

  it("loads a deep-linked chunk outside the first page without adding it to bulk selection", async () => {
    h.target = {
      ...sampleChunks[0],
      id: "outside",
      content: "referenced chunk from another page",
      positions: [[4, 1, 20, 3, 40]],
    };
    const user = userEvent.setup();
    renderPage("/knowledge/kb1/documents/d1/chunks?chunkId=outside");
    expect(
      screen.getByText("referenced chunk from another page"),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("img", { name: "PDF original" })).toHaveAttribute(
        "data-page",
        "4",
      ),
    );
    await user.click(screen.getByRole("checkbox", { name: "选择本页" }));
    await user.click(screen.getByRole("button", { name: "批量停用切片" }));
    expect(h.switchMock).toHaveBeenCalledWith({
      chunkIds: ["c1", "c2"],
      available: false,
    });
  });

  it("shows a protected image for a read-only deep link outside the current page", async () => {
    setAuthRole("member");
    h.target = {
      ...sampleChunks[1],
      id: "outside-image",
      image_id: "target-image",
      content: "Image source reference",
    };
    renderPage("/knowledge/kb1/documents/d1/chunks?chunkId=outside-image");
    expect(screen.getByText("Image source reference")).toBeInTheDocument();
    await waitFor(() => {
      expect(h.fetchImageMock).toHaveBeenCalledWith(
        "kb1",
        "target-image",
        expect.any(AbortSignal),
      );
    });
    expect(
      screen.queryByRole("button", { name: "新增切片" }),
    ).not.toBeInTheDocument();
    expect(screen.queryAllByRole("button", { name: /编辑/ })).toHaveLength(0);
  });

  it("member: hides editor/switch/delete/bulk but keeps data and copy ID", () => {
    setAuthRole("member");
    renderPage();

    // 数据仍可见（只读）
    expect(screen.getByText("original chunk content")).toBeInTheDocument();
    expect(screen.getByText("文档信息")).toBeInTheDocument();
    // 写操作按钮隐藏：新增切片/编辑/删除/启用 Switch
    expect(
      screen.queryByRole("button", { name: "新增切片" }),
    ).not.toBeInTheDocument();
    expect(screen.queryAllByRole("button", { name: /编辑/ })).toHaveLength(0);
    expect(screen.queryAllByRole("button", { name: /删除/ })).toHaveLength(0);
    expect(document.querySelector(".ant-switch")).toBeNull();
    // 批量勾选与批量栏不存在
    expect(
      screen.queryByRole("checkbox", { name: "选择本页" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/已选择/)).not.toBeInTheDocument();
  });
});
