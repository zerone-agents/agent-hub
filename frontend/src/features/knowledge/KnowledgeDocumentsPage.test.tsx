import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router";
import { ConfigProvider } from "antd";
import { antdTheme } from "@/lib/antd-theme";
import KnowledgeDocumentsPage from "./KnowledgeDocumentsPage";
import { setAuthRole } from "@/test/auth-store-mock";

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock("@/stores/auth", async () =>
  (await import("@/test/auth-store-mock")).createAuthStoreMock(),
);

const h = vi.hoisted(() => ({
  paramsMock: vi.fn(),
  documents: [] as Record<string, unknown>[],
  total: 0,
  refetchMock: vi.fn(),
  uploadMock: vi.fn(),
  parseMock: vi.fn(),
  ingestMock: vi.fn(),
  stopMock: vi.fn(),
  updateMock: vi.fn(),
  statusMock: vi.fn(),
  deleteMock: vi.fn(),
  downloadMock: vi.fn(),
}));

vi.mock("@/queries/useKnowledge", () => ({
  useDocuments: (_id: string, params: unknown) => {
    h.paramsMock(params);
    return {
      data: { documents: h.documents, total: h.total },
      isLoading: false,
      isFetching: false,
      refetch: h.refetchMock,
    };
  },
  useDocumentFilters: () => ({
    data: {
      total: 2,
      filter: {
        suffix: { pdf: 1 },
        run_status: { "3": 1 },
        metadata: { owner: { team: 1 } },
      },
    },
    refetch: vi.fn(),
  }),
  useUploadDocuments: () => ({ mutateAsync: h.uploadMock, isPending: false }),
  useIngestDocuments: () => ({ mutateAsync: h.ingestMock }),
  useParseDocuments: () => ({ mutate: h.parseMock }),
  useStopParsingDocuments: () => ({ mutate: h.stopMock }),
  useUpdateDocument: () => ({
    mutate: h.updateMock,
    mutateAsync: h.updateMock,
    isPending: false,
  }),
  useDeleteDocuments: () => ({
    mutate: h.deleteMock,
    mutateAsync: h.deleteMock,
  }),
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

vi.mock("@/api/knowledgeManagement", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/knowledgeManagement")>()),
  knowledgeManagement: { documentStatus: h.statusMock },
}));

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
    setAuthRole("admin");
    h.documents = sampleDocs;
    h.total = sampleDocs.length;
    h.paramsMock.mockReset();
    h.refetchMock.mockReset();
    h.uploadMock.mockReset();
    h.uploadMock.mockResolvedValue([{ id: "new-doc" }]);
    h.ingestMock.mockReset();
    h.ingestMock.mockResolvedValue(true);
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
    expect(screen.getByText("添加资料")).toBeInTheDocument();
    expect(screen.getAllByText("元数据").length).toBeGreaterThan(0);
  }, 15000);

  it("queues files in upload modal and auto parses uploaded docs", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("button", { name: "添加资料" }));
    await user.click(screen.getByRole("menuitem", { name: "上传文档" }));
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

    await waitFor(() => {
      expect(h.uploadMock).toHaveBeenCalledWith([expect.any(File)]);
    });
    await waitFor(() => {
      expect(h.ingestMock).toHaveBeenCalledWith({
        doc_ids: ["new-doc"],
        run: 1,
        delete: false,
        apply_kb: false,
      });
    });
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
    await user.click(screen.getByRole("button", { name: "提交解析请求" }));
    await waitFor(() => {
      expect(h.ingestMock).toHaveBeenCalledWith({
        doc_ids: ["d1"],
        run: 1,
        delete: false,
        apply_kb: false,
      });
    });

    await user.click(
      screen.getByRole("button", { name: "停止解析 running.docx" }),
    );
    expect(h.stopMock).toHaveBeenCalledWith(["d2"]);
  });

  it("downloads documents through the authenticated API client", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(
      screen.getByRole("button", { name: "更多文档操作: guide.pdf" }),
    );
    const button = await screen.findByRole("menuitem", { name: /下载/ });
    expect(button).not.toHaveAttribute("href");

    await user.click(button);

    await waitFor(() => {
      expect(h.downloadMock).toHaveBeenCalledWith("kb1", "d1");
    });
    expect(URL.createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalled();
    await waitFor(() => {
      expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:download");
    });
  });

  it("supports bulk enable and parse", async () => {
    h.statusMock.mockResolvedValueOnce({ succeeded: ["d1", "d2"], failed: [] });
    const user = userEvent.setup();
    renderPage();

    const checkboxes = screen.getAllByRole("checkbox");
    await user.click(checkboxes[1]);
    await user.click(checkboxes[2]);

    await screen.findByText("已选择 2 个文档");
    await user.click(screen.getByRole("button", { name: "批量停用文档" }));
    await waitFor(() => {
      expect(h.statusMock).toHaveBeenCalledWith("kb1", ["d1", "d2"], false);
    });

    const nextCheckboxes = screen.getAllByRole("checkbox");
    await user.click(nextCheckboxes[1]);
    await user.click(screen.getByRole("button", { name: "批量解析文档" }));
    await user.click(
      screen.getByRole("checkbox", { name: "清除旧切片后重新解析" }),
    );
    await user.click(
      screen.getByRole("checkbox", { name: "应用知识库元数据模板" }),
    );
    await user.click(screen.getByRole("button", { name: "提交解析请求" }));
    await waitFor(() => {
      expect(h.ingestMock).toHaveBeenCalledWith({
        doc_ids: ["d1"],
        run: 1,
        delete: true,
        apply_kb: true,
      });
    });
  }, 15000);

  it("keeps only failed documents selected after a partial bulk operation", async () => {
    h.statusMock.mockResolvedValueOnce({ succeeded: ["d1"], failed: ["d2"] });
    const user = userEvent.setup();
    renderPage();
    const boxes = screen.getAllByRole("checkbox");
    await user.click(boxes[1]);
    await user.click(boxes[2]);
    await user.click(screen.getByRole("button", { name: "批量停用文档" }));
    await screen.findByText("已选择 1 个文档");
    expect(screen.getByText(/1 项未完成；失败项/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "批量解析文档" }));
    await user.click(screen.getByRole("button", { name: "提交解析请求" }));
    await waitFor(() => {
      expect(h.ingestMock).toHaveBeenCalledWith(
        expect.objectContaining({ doc_ids: ["d2"] }),
      );
    });
  });

  it("member: hides upload/parse/rename/delete/bulk but keeps download and view", async () => {
    setAuthRole("member");
    const user = userEvent.setup();
    renderPage();

    // 数据仍可见（只读）
    expect(screen.getByText("guide.pdf")).toBeInTheDocument();
    expect(screen.getByText("running.docx")).toBeInTheDocument();
    // 只读操作保留：下载、切片
    expect(
      screen.getByRole("button", { name: "更多文档操作: guide.pdf" }),
    ).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "切片" }).length).toBe(2);
    // 查看解析状态保留
    expect(
      screen.getByRole("button", { name: "查看解析状态：解析中" }),
    ).toBeInTheDocument();

    // 写操作按钮隐藏
    expect(screen.queryByText("添加资料")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "解析文档 guide.pdf" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "停止解析 running.docx" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "重命名" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "删除" }),
    ).not.toBeInTheDocument();
    // 批量勾选与批量栏不存在
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
    expect(screen.queryByText(/已选择/)).not.toBeInTheDocument();
    // 启用列只读展示，不再渲染 Switch
    expect(document.querySelector(".ant-switch")).toBeNull();

    // 下载功能对 member 仍然可用
    await user.click(
      screen.getByRole("button", { name: "更多文档操作: guide.pdf" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: /下载/ }));
    await waitFor(() => {
      expect(h.downloadMock).toHaveBeenCalledWith("kb1", "d1");
    });
  }, 15000);

  it("keeps reparse options open when acceptance fails", async () => {
    h.ingestMock.mockRejectedValueOnce(new Error("not accepted"));
    const user = userEvent.setup();
    renderPage();
    await user.click(
      screen.getByRole("button", { name: "解析文档 guide.pdf" }),
    );
    await user.click(
      screen.getByRole("checkbox", { name: "清除旧切片后重新解析" }),
    );
    await user.click(screen.getByRole("button", { name: "提交解析请求" }));
    await screen.findByText("not accepted");
    expect(
      screen.getByRole("dialog", { name: "解析 1 个文档" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "清除旧切片后重新解析" }),
    ).toBeChecked();
  });

  it("makes empty-metadata mode override other metadata filters while keeping their drafts", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("button", { name: /精细文档筛选/ }));
    const composite =
      screen.getByPlaceholderText("元数据筛选条件（JSON 对象）");
    const condition =
      '{"conditions":[{"name":"version","comparison_operator":"=","value":"v2"}]}';
    await user.click(composite);
    await user.paste(condition);
    await user.keyboard("{Enter}");
    await user.click(screen.getByRole("tab", { name: "高级 JSON" }));
    const exact = screen.getByRole("textbox", { name: "高级 JSON" });
    await user.click(exact);
    await user.clear(exact);
    await user.paste('{"owner":"team"}');
    await user.click(screen.getByRole("button", { name: "应用筛选" }));
    await user.click(
      screen.getByRole("switch", { name: "仅显示无元数据文档" }),
    );
    expect(h.paramsMock).toHaveBeenLastCalledWith(
      expect.objectContaining({
        return_empty_metadata: true,
        metadata_condition: undefined,
        metadata: undefined,
      }),
    );
    expect(composite).toHaveValue(condition);
    expect(composite).toBeDisabled();
    await user.click(
      screen.getByRole("switch", { name: "仅显示无元数据文档" }),
    );
    expect(h.paramsMock).toHaveBeenLastCalledWith(
      expect.objectContaining({
        return_empty_metadata: false,
        metadata_condition: condition,
        metadata: '{"owner":["team"]}',
      }),
    );
  });
});

it("applies metadata from the visual candidate and value editor", async () => {
  setAuthRole("admin");
  h.documents = sampleDocs;
  h.total = 2;
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("button", { name: /精细文档筛选/ }));
  await user.click(screen.getByRole("combobox", { name: "添加字段" }));
  await user.click(
    screen.getByText("owner", { selector: ".ant-select-item-option-content" }),
  );
  await user.click(screen.getByRole("button", { name: "添加字段" }));
  await user.click(screen.getByRole("combobox", { name: "新值: owner" }));
  await user.click(
    screen.getByText("team", { selector: ".ant-select-item-option-content" }),
  );
  await user.click(screen.getByRole("button", { name: "应用筛选" }));
  expect(h.paramsMock).toHaveBeenLastCalledWith(
    expect.objectContaining({ metadata: '{"owner":["team"]}', page: 1 }),
  );
});
it("preserves zero and false filters and refuses a predicate the server would discard", async () => {
  setAuthRole("admin");
  h.documents = sampleDocs;
  h.total = 2;
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("button", { name: /精细文档筛选/ }));
  await user.click(screen.getByRole("tab", { name: "高级 JSON" }));
  const input = screen.getByRole("textbox", { name: "高级 JSON" });
  await user.clear(input);
  await user.click(input);
  await user.paste('{"revision":0,"active":false}');
  await user.click(screen.getByRole("button", { name: "应用筛选" }));
  expect(h.paramsMock).toHaveBeenLastCalledWith(
    expect.objectContaining({ metadata: '{"revision":[0],"active":[false]}' }),
  );
  await user.clear(input);
  await user.click(input);
  await user.paste('{"revision":[]}');
  await user.click(screen.getByRole("button", { name: "应用筛选" }));
  expect(h.paramsMock).toHaveBeenLastCalledWith(
    expect.objectContaining({ metadata: '{"revision":[0],"active":[false]}' }),
  );
});
it("retains special exact-filter keys and keeps the effective filter and draft after numeric overflow", async () => {
  setAuthRole("admin");
  h.documents = sampleDocs;
  h.total = 2;
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("button", { name: /精细文档筛选/ }));
  await user.click(screen.getByRole("tab", { name: "高级 JSON" }));
  const input = screen.getByRole("textbox", { name: "高级 JSON" });
  await user.clear(input);
  await user.click(input);
  await user.paste('{"__proto__":"x","revision":0}');
  await user.click(screen.getByRole("button", { name: "应用筛选" }));
  const effective = '{"__proto__":["x"],"revision":[0]}';
  expect(h.paramsMock).toHaveBeenLastCalledWith(
    expect.objectContaining({ metadata: effective }),
  );
  const draft = '{"revision":1e309}';
  await user.clear(input);
  await user.click(input);
  await user.paste(draft);
  await user.click(screen.getByRole("button", { name: "应用筛选" }));
  expect(h.paramsMock).toHaveBeenLastCalledWith(
    expect.objectContaining({ metadata: effective }),
  );
  expect(input).toHaveValue(draft);
  expect(
    await screen.findByText(
      "精确筛选的数值必须是有限数字；请修正草稿后再应用。",
    ),
  ).toBeInTheDocument();
});
