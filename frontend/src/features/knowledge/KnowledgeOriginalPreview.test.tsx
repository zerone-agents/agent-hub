import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { ConfigProvider } from "antd";
import KnowledgeOriginalPreview, {
  chunkPositions,
} from "./KnowledgeOriginalPreview";
import type { KnowledgeChunk, KnowledgeDocument } from "@/api/knowledge";

const h = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("@/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/client")>();
  return {
    ...actual,
    default: {
      ...actual.default,
      get: async (...args: unknown[]) => {
        const response = (await h.get(...args)) as { data: Blob };
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

const doc = { id: "d1", name: "guide.pdf" } as KnowledgeDocument;
const chunk = {
  id: "c1",
  content: "exact source",
  positions: [
    [3, 10, 40, 20, 50],
    [7, 0, 20, 10, 30],
  ],
} as KnowledgeChunk;
function mount(document = doc, selectedChunk: KnowledgeChunk | null = chunk) {
  return render(
    <ConfigProvider>
      <KnowledgeOriginalPreview
        datasetId="kb1"
        documentId={document.id}
        document={document}
        selectedChunk={selectedChunk}
      />
    </ConfigProvider>,
  );
}
function readableBlob(content: string, type: string) {
  return new Blob([content], { type });
}
beforeEach(() => {
  h.get.mockReset();
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
  Object.defineProperty(Blob.prototype, "text", {
    configurable: true,
    value: function (this: Blob) {
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
          resolve(reader.result);
        };
        reader.onerror = reject;
        reader.readAsText(this);
      });
    },
  });
  Object.defineProperty(URL, "createObjectURL", {
    configurable: true,
    value: vi.fn(() => "blob:original"),
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    configurable: true,
    value: vi.fn(),
  });
  h.get.mockResolvedValue({
    data: readableBlob("%PDF-1.7", "application/pdf"),
  });
});
describe("KnowledgeOriginalPreview", () => {
  it("uses the protected download with cancellation and jumps to actual 1-based page positions", async () => {
    const { unmount } = mount();
    const iframe = await screen.findByRole("img", { name: "PDF original" });
    expect(iframe).toHaveAttribute("data-page", "3");
    expect(h.get).toHaveBeenCalledWith(
      "/api/v1/admin/knowledge/datasets/kb1/documents/d1/download",
      expect.objectContaining({
        signal: expect.any(AbortSignal),
        responseType: "stream",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "页码 7 · 0, 20, 10, 30" }),
    );
    expect(screen.getByRole("img", { name: "PDF original" })).toHaveAttribute(
      "data-page",
      "7",
    );
    const signal = h.get.mock.calls[0][1].signal as AbortSignal;
    unmount();
    expect(signal.aborted).toBe(true);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:original");
  });
  it("does not guess malformed or scalar legacy positions", () => {
    expect(
      chunkPositions({
        positions: [
          1,
          [0, 1, 2, 3, 4],
          [1, 4, 2, 1, 2],
          [2, 1, 2, 3, 4],
          [1, 2],
        ],
      } as KnowledgeChunk),
    ).toEqual([{ page: 2, coordinates: [1, 2, 3, 4] }]);
  });
  it("blocks an error envelope masquerading as PDF and offers retry without blocking chunks", async () => {
    h.get.mockResolvedValueOnce({
      data: readableBlob('{"success":false}', "application/json"),
    });
    mount();
    expect(await screen.findByText("原文加载失败")).toBeInTheDocument();
    expect(
      screen.queryByRole("img", { name: "PDF original" }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /重\s*试/ }));
    expect(
      await screen.findByRole("img", { name: "PDF original" }),
    ).toHaveAttribute("data-page", "3");
  });
  it("does not automatically download a document known to exceed the preview limit", async () => {
    mount({ ...doc, size: 51 * 1024 * 1024 });
    expect(
      await screen.findByText("原文超过预览大小限制（50 MB），请下载查看。"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "下载原文" })).toBeEnabled();
    expect(h.get).not.toHaveBeenCalled();
  });

  it("keeps office formats as an explicit download fallback without automatic fetch", () => {
    mount({ ...doc, name: "report.docx" });
    expect(
      screen.getByText("此格式暂不支持预览，请下载原文核对。"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "下载原文" })).toBeEnabled();
    expect(h.get).not.toHaveBeenCalled();
  });
  it("renders text as plain text and marks the first exact match without executing markup", async () => {
    h.get.mockResolvedValue({
      data: readableBlob(
        "<script>alert(1)</script>\nexact source\nother",
        "text/plain",
      ),
    });
    mount({ ...doc, name: "notes.txt" });
    expect(
      await screen.findByText("已找到切片文本在原文中的首次匹配"),
    ).toBeInTheDocument();
    expect(document.querySelector("mark")).toHaveTextContent("exact source");
    expect(document.querySelector("pre script")).toBeNull();
    expect(screen.getByText(/<script>alert/)).toBeInTheDocument();
  });
  it("aborts a changed document and ignores the previous response even when the server returns late", async () => {
    let resolveOld!: (value: { data: Blob }) => void;
    h.get.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    );
    h.get.mockResolvedValueOnce({
      data: readableBlob("new original", "text/plain"),
    });
    const { rerender } = mount({ ...doc, name: "old.txt" });
    const oldSignal = h.get.mock.calls[0][1].signal as AbortSignal;
    rerender(
      <ConfigProvider>
        <KnowledgeOriginalPreview
          datasetId="kb1"
          documentId="d2"
          document={{ ...doc, id: "d2", name: "new.txt" }}
        />
      </ConfigProvider>,
    );
    expect(await screen.findByText("new original")).toBeInTheDocument();
    expect(oldSignal.aborted).toBe(true);
    resolveOld({ data: readableBlob("old original", "text/plain") });
    await waitFor(() => {
      expect(URL.createObjectURL).toHaveBeenCalledTimes(1);
    });
    expect(screen.queryByText("old original")).not.toBeInTheDocument();
  });
  it("renders an image preview using its owned Blob URL", async () => {
    h.get.mockResolvedValue({
      data: new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], {
        type: "image/png",
      }),
    });
    mount({ ...doc, name: "scan.png" }, null);
    expect(
      await screen.findByRole("img", { name: "scan.png" }),
    ).toHaveAttribute("src", "blob:original");
  });
});
