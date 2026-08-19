import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router";
import { ConfigProvider } from "antd";
import { antdTheme } from "@/lib/antd-theme";
import KnowledgeDocumentsPage from "./KnowledgeDocumentsPage";

const h = vi.hoisted(() => ({
  documents: [] as Record<string, unknown>[],
  total: 0,
  refetchMock: vi.fn(),
  uploadMock: vi.fn(),
  parseMock: vi.fn(),
  stopMock: vi.fn(),
  updateMock: vi.fn(),
  deleteMock: vi.fn(),
  downloadMock: vi.fn(),
  // 角色默认 admin：既有断言依赖上传/解析/批量等写按钮可见；member 分支用例内切换。
  user: { id: "1", name: "admin", email: "admin@zerone.run", role: "admin" },
}));

vi.mock("@/stores/auth", () => ({
  useAuthStore: (selector: (s: {
    user: { id: string; name: string; email: string; role: string } | null
    setUser: () => void
    loginWithPassword: () => Promise<void>
    login: () => void
    logout: () => Promise<void>
  }) => unknown) => selector({
    user: h.user,
    setUser: vi.fn(),
    loginWithPassword: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(),
  }),
}));

vi.mock("@/queries/useKnowledge", () => ({
  useDocuments: () => ({
    data: { documents: h.documents, total: h.total },
    isLoading: false,
    isFetching: false,
    refetch: h.refetchMock,
  }),
  useUploadDocuments: () => ({ mutateAsync: h.uploadMock, isPending: false }),
  useParseDocuments: () => ({ mutate: h.parseMock }),
  useStopParsingDocuments: () => ({ mutate: h.stopMock }),
  useUpdateDocument: () => ({
    mutate: h.updateMock,
    mutateAsync: h.updateMock,
    isPending: false,
  }),
  useDeleteDocuments: () => ({ mutate: h.deleteMock }),
}));

vi.mock("@/api/knowledge", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/knowledge")>();
  return {
    ...actual,
    knowledgeApi: {
      ...actual.knowledgeApi,
      documents: {
        ...actual.knowledgeApi.documents,
        download: h.downloadMock,
      },
    },
  };
});

const sampleDocs = [
  {
    id: "d1",
    name: "guide.pdf",
    chunk_num: 12,
    token_num: 900,
    parser_id: "naive",
    run: "0",
    progress: 0,
    progress_msg: "",
    status: "1",
    enabled: true,
    size: 2048,
    type: "pdf",
    suffix: "pdf",
    meta_fields: [{ key: "author" }],
    parser_config: {},
  },
  {
    id: "d2",
    name: "running.docx",
    chunk_num: 2,
    token_num: 90,
    parser_id: "qa",
    run: "1",
    progress: 0.5,
    progress_msg: "extracting",
    status: "1",
    enabled: true,
    size: 1024,
    type: "docx",
    suffix: "docx",
    meta_fields: [],
    parser_config: {},
  },
];

function renderPage() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={["/knowledge/kb1/documents"]}>
        <Routes>
          <Route
            path="/knowledge/:id/documents"
            element={<KnowledgeDocumentsPage />}
          />
        </Routes>
      </MemoryRouter>
    </ConfigProvider>,
  );
}

describe("KnowledgeDocumentsPage", () => {
  beforeEach(() => {
    h.user = { ...h.user, role: "admin" };
    h.documents = sampleDocs;
    h.total = sampleDocs.length;
    h.refetchMock.mockReset();
    h.uploadMock.mockReset();
    h.uploadMock.mockResolvedValue([{ id: "new-doc" }]);
    h.parseMock.mockReset();
    h.stopMock.mockReset();
    h.updateMock.mockReset();
    h.updateMock.mockResolvedValue({});
    h.deleteMock.mockReset();
    h.downloadMock.mockReset();
    h.downloadMock.mockResolvedValue({
      data: new Blob(["pdf"], { type: "application/pdf" }),
      headers: { "content-disposition": 'attachment; filename="guide.pdf"' },
    });
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: vi.fn(() => "blob:download"),
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: vi.fn(),
    });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
      () => undefined,
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the document console rows", () => {
    renderPage();
    expect(screen.getByText("guide.pdf")).toBeInTheDocument();
    expect(screen.getByText("running.docx")).toBeInTheDocument();
    expect(screen.getByText("上传文档")).toBeInTheDocument();
    expect(screen.getAllByText("Metadata").length).toBeGreaterThan(0);
  }, 15000);

  it("queues files in upload modal and auto parses uploaded docs", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("button", { name: "上传文档" }));
    const modal = screen.getByRole("dialog", { name: "上传文档" });
    const input = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    await user.upload(
      input,
      new File(["hello"], "a.pdf", { type: "application/pdf" }),
    );

    expect(within(modal).getByText("a.pdf")).toBeInTheDocument();
    await user.click(within(modal).getByRole("button", { name: "开始上传" }));

    await waitFor(() =>
      { expect(h.uploadMock).toHaveBeenCalledWith([expect.any(File)]); },
    );
    expect(h.parseMock).toHaveBeenCalledWith(["new-doc"]);
  });

  it("opens status details for a running document", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(
      screen.getByRole("button", { name: "查看解析状态：解析中" }),
    );

    expect(
      await screen.findByRole("dialog", { name: "解析状态" }),
    ).toBeInTheDocument();
    expect(screen.getByText("extracting")).toBeInTheDocument();
  });

  it("triggers row parse and stop actions", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(
      screen.getByRole("button", { name: "解析文档 guide.pdf" }),
    );
    expect(h.parseMock).toHaveBeenCalledWith(["d1"]);

    await user.click(
      screen.getByRole("button", { name: "停止解析 running.docx" }),
    );
    expect(h.stopMock).toHaveBeenCalledWith(["d2"]);
  });

  it("downloads documents through the authenticated API client", async () => {
    const user = userEvent.setup();
    renderPage();

    const button = screen.getByRole("button", { name: "下载 guide.pdf" });
    expect(button).not.toHaveAttribute("href");

    await user.click(button);

    await waitFor(() =>
      { expect(h.downloadMock).toHaveBeenCalledWith("kb1", "d1"); },
    );
    expect(URL.createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalled();
    await waitFor(() =>
      { expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:download"); },
    );
  });

  it("supports bulk enable and parse", async () => {
    const user = userEvent.setup();
    renderPage();

    const checkboxes = screen.getAllByRole("checkbox");
    await user.click(checkboxes[1]);
    await user.click(checkboxes[2]);

    await screen.findByText("已选择 2 个文档");
    await user.click(screen.getByRole("button", { name: "批量停用文档" }));
    await waitFor(() => { expect(h.updateMock).toHaveBeenCalledTimes(2); });

    const nextCheckboxes = screen.getAllByRole("checkbox");
    await user.click(nextCheckboxes[1]);
    await user.click(screen.getByRole("button", { name: "批量解析文档" }));
    expect(h.parseMock).toHaveBeenCalledWith(["d1"]);
  }, 15000);

  it("member: hides upload/parse/rename/delete/bulk but keeps download and view", async () => {
    h.user = { ...h.user, role: "member" };
    const user = userEvent.setup();
    renderPage();

    // 数据仍可见（只读）
    expect(screen.getByText("guide.pdf")).toBeInTheDocument();
    expect(screen.getByText("running.docx")).toBeInTheDocument();
    // 只读操作保留：下载、切片
    expect(screen.getByRole("button", { name: "下载 guide.pdf" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "切片" }).length).toBe(2);
    // 查看解析状态保留
    expect(
      screen.getByRole("button", { name: "查看解析状态：解析中" }),
    ).toBeInTheDocument();

    // 写操作按钮隐藏
    expect(screen.queryByText("上传文档")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "解析文档 guide.pdf" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "停止解析 running.docx" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重命名" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "删除" })).not.toBeInTheDocument();
    // 批量勾选与批量栏不存在
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
    expect(screen.queryByText(/已选择/)).not.toBeInTheDocument();
    // 启用列只读展示，不再渲染 Switch
    expect(document.querySelector(".ant-switch")).toBeNull();

    // 下载功能对 member 仍然可用
    await user.click(screen.getByRole("button", { name: "下载 guide.pdf" }));
    await waitFor(() =>
      { expect(h.downloadMock).toHaveBeenCalledWith("kb1", "d1"); },
    );
  }, 15000);
});
