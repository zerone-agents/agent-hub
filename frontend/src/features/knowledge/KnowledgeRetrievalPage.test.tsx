import { describe, it, expect, beforeEach, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router";
import { ConfigProvider } from "antd";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { antdTheme } from "@/lib/antd-theme";
import { setAuthRole } from "@/test/auth-store-mock";
import KnowledgeRetrievalPage, {
  HighlightedContent,
} from "./KnowledgeRetrievalPage";

const h = vi.hoisted(() => ({
  retrievalMock: vi.fn(),
  documentsMock: vi.fn(),
  modelsMock: vi.fn(),
  metadataKeysMock: vi.fn(),
  error: undefined as unknown,
  data: undefined as unknown,
}));

vi.mock("@/queries/useKnowledge", () => ({
  useDocuments: () => ({
    data: { total: 1, documents: [{ id: "d1", name: "guide.pdf" }] },
  }),
  useRetrievalTest: () => ({
    mutate: h.retrievalMock,
    isPending: false,
    error: h.error,
    data: h.data,
  }),
}));

vi.mock("@/stores/auth", async () =>
  (await import("@/test/auth-store-mock")).createAuthStoreMock(),
);

vi.mock("@/api/knowledge", () => ({
  knowledgeApi: { documents: { list: h.documentsMock } },
}));
vi.mock("@/api/knowledgeManagement", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/knowledgeManagement")>()),
  knowledgeMetadataApi: { keys: h.metadataKeysMock },
}));
vi.mock("@/queries/useMultirag", () => ({ useMultiragModels: h.modelsMock }));

const sampleResult = {
  total: 1,
  chunks: [
    {
      id: "c1",
      content: "matched chunk text",
      document_id: "d1",
      document_name: "guide.pdf",
      similarity: 0.873,
      vector_similarity: 0.9,
      term_similarity: 0.8,
    },
  ],
  doc_aggs: [],
  labels: {},
};

function renderPage() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter initialEntries={["/knowledge/kb1/retrieval"]}>
          <Routes>
            <Route
              path="/knowledge/:id/retrieval"
              element={<KnowledgeRetrievalPage />}
            />
          </Routes>
        </MemoryRouter>
      </ConfigProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  setAuthRole("admin");
  h.retrievalMock.mockReset();
  h.error = undefined;
  h.data = undefined;
  h.metadataKeysMock.mockReset().mockResolvedValue(["version", "category"]);
  h.documentsMock.mockReset().mockResolvedValue({
    total: 1,
    documents: [{ id: "d1", name: "guide.pdf" }],
  });
  h.modelsMock.mockReturnValue({
    data: [
      {
        name: "bge-reranker",
        fullId: "bge-reranker@API",
        factory: "API",
        type: "rerank",
        status: "1",
      },
    ],
    isPending: false,
    isFetching: false,
  });
});

describe("KnowledgeRetrievalPage", () => {
  beforeEach(() => {
    setAuthRole("admin");
    h.retrievalMock.mockReset();
    h.error = undefined;
    h.data = undefined;
  });

  it("submits the retrieval form with the dataset id", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.type(
      screen.getByRole("textbox", { name: "检索问题" }),
      "如何退货？",
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));

    await waitFor(() => {
      expect(h.retrievalMock).toHaveBeenCalled();
    });
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({ question: "如何退货？", dataset_ids: ["kb1"] }),
    );
  });

  it("renders retrieval results", () => {
    h.data = sampleResult;
    renderPage();
    expect(screen.getByText("共召回 1 条分块")).toBeInTheDocument();
    expect(screen.getByText("matched chunk text")).toBeInTheDocument();
    expect(screen.getByText("guide.pdf")).toBeInTheDocument();
    expect(screen.getByText(/相似度 0.873/)).toBeInTheDocument();
  });

  it("member: hides the retrieval test submit button but keeps results visible", () => {
    setAuthRole("member");
    h.data = sampleResult;
    renderPage();

    // 写操作入口（检索测试）隐藏；已渲染的检索结果区不受影响
    expect(
      screen.queryByRole("button", { name: /检索测试/ }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("共召回 1 条分块")).toBeInTheDocument();
    expect(screen.getByText("matched chunk text")).toBeInTheDocument();
  });
});

it("renders emphasis without upstream active HTML", () => {
  const { container } = render(
    <HighlightedContent
      content={
        "Hello <em>world</em><img src=x onerror=alert(1)><script>bad()</script>"
      }
    />,
  );
  expect(container.querySelector("mark")).toHaveTextContent("world");
  expect(container.querySelector("img")).toBeNull();
  expect(container.querySelector("script")).toBeNull();
  expect(container).not.toHaveTextContent("bad()");
});

it("explains selected-document threshold bypass without changing the request or hiding zero scores", async () => {
  h.data = {
    ...sampleResult,
    chunks: [{ ...sampleResult.chunks[0], similarity: 0 }],
  };
  const user = userEvent.setup();
  const view = renderPage();
  const hint = /已选择文档时，服务不会按此阈值过滤最终得分/;
  expect(screen.queryByText(hint)).not.toBeInTheDocument();
  await user.click(screen.getByRole("combobox", { name: "文档范围" }));
  await user.click(
    await screen.findByText("guide.pdf", {
      selector: ".ant-select-item-option-content",
    }),
  );
  await user.keyboard("{Escape}");
  expect(screen.getByText(hint)).toBeInTheDocument();
  expect(screen.getByRole("spinbutton", { name: "相似度阈值" })).toBeEnabled();
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Question",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({ doc_ids: ["d1"], similarity_threshold: 0.2 }),
    );
  });
  expect(screen.getByText("相似度 0.000")).toBeInTheDocument();
  const remove = view.container.querySelector(
    '.ant-select-selection-item[title="guide.pdf"] .ant-select-selection-item-remove',
  );
  expect(remove).not.toBeNull();
  await user.click(remove!);
  expect(screen.queryByText(hint)).not.toBeInTheDocument();
  expect(screen.getByText("相似度 0.000")).toBeInTheDocument();
});

it("sends manual metadata with canonical operators and omits empty document scope", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Version?",
  );
  await user.click(screen.getByRole("button", { name: /高级 JSON/ }));
  const input = screen.getByRole("textbox", { name: "高级 JSON" });
  await user.click(input);
  await user.paste(
    '{"logic":"and","conditions":[{"name":"version","comparison_operator":"is","value":"v2"}]}',
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalled();
  });
  const request = h.retrievalMock.mock.calls[0][0];
  expect(request.doc_ids).toBeUndefined();
  expect(request.meta_data_filter).toEqual({
    method: "manual",
    logic: "and",
    manual: [{ key: "version", op: "=", value: "v2" }],
  });
  expect(request.reference_metadata).toEqual({ include: true });
  expect(request).toMatchObject({ page: 1, size: 30 });
});
it("renders document metadata as text and keeps zero matches inside the requested scope", () => {
  h.data = {
    ...sampleResult,
    chunks: [
      {
        ...sampleResult.chunks[0],
        document_metadata: {
          version: "v2",
          owner: "<img src=x onerror=bad()>",
        },
      },
    ],
  };
  const view = renderPage();
  expect(screen.getByLabelText("文档元数据")).toHaveTextContent("version: v2");
  expect(screen.getByLabelText("文档元数据").querySelector("img")).toBeNull();
  view.unmount();
  h.data = { ...sampleResult, total: 0, chunks: [] };
  renderPage();
  expect(screen.getByText(/不会自动扩大到全库/)).toBeInTheDocument();
  expect(h.retrievalMock).not.toHaveBeenCalled();
});
it("keeps server filter errors visible and hides stale previous results", () => {
  h.error = new Error("文档范围与元数据过滤的交集尚待上游联验");
  h.data = sampleResult;
  renderPage();
  expect(
    screen.getByText("文档范围与元数据过滤的交集尚待上游联验"),
  ).toBeInTheDocument();
  expect(screen.queryByText("matched chunk text")).not.toBeInTheDocument();
});

it("blocks graph retrieval from expanding metadata scope without clearing the draft", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Version?",
  );
  await user.click(screen.getByRole("button", { name: /检索策略与模型/ }));
  await user.click(screen.getByRole("switch", { name: "结合知识图谱" }));
  await user.click(screen.getByRole("button", { name: /高级 JSON/ }));
  const input = screen.getByRole("textbox", { name: "高级 JSON" });
  const filter =
    '{"conditions":[{"name":"version","comparison_operator":"=","value":"v2"}]}';
  await user.click(input);
  await user.paste(filter);
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await screen.findByText(/知识图谱会扩展来源/);
  expect(h.retrievalMock).not.toHaveBeenCalled();
  expect(input).toHaveValue(filter);
  expect(screen.getByRole("switch", { name: "结合知识图谱" })).toBeChecked();
});

it("allows document scope and metadata intersection after gateway verification", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Version?",
  );
  await user.click(screen.getByRole("combobox", { name: "文档范围" }));
  await user.click(await screen.findByText("guide.pdf"));
  await user.click(screen.getByRole("button", { name: /高级 JSON/ }));
  const input = screen.getByRole("textbox", { name: "高级 JSON" });
  await user.click(input);
  await user.paste(
    '{"conditions":[{"name":"version","comparison_operator":"=","value":"v2"}]}',
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({
        doc_ids: ["d1"],
        meta_data_filter: expect.objectContaining({ method: "manual" }),
      }),
    );
  });
});

it("loads beyond 100 documents and keeps selection through a remote search", async () => {
  h.documentsMock.mockImplementation(async (_dataset, params) => {
    if (params.keywords === "tail")
      return { total: 1, documents: [{ id: "d101", name: "tail.pdf" }] };
    return {
      total: 101,
      documents:
        params.page === 1
          ? Array.from({ length: 100 }, (_, i) => ({
              id: `d${i + 1}`,
              name: `file-${i + 1}.pdf`,
            }))
          : [{ id: "d101", name: "tail.pdf" }],
    };
  });
  const user = userEvent.setup();
  renderPage();
  const scope = screen.getByRole("combobox", { name: "文档范围" });
  await user.click(scope);
  await user.click(await screen.findByText("file-1.pdf"));
  await user.click(screen.getByRole("button", { name: "加载更多文档" }));
  await screen.findByText("已加载 101 / 101 个文档");
  expect(h.documentsMock).toHaveBeenCalledWith("kb1", {
    page: 2,
    page_size: 100,
    keywords: "",
  });
  await user.type(scope, "tail");
  await waitFor(() => {
    expect(h.documentsMock).toHaveBeenCalledWith("kb1", {
      page: 1,
      page_size: 100,
      keywords: "tail",
    });
  });
  await user.click(await screen.findByText("tail.pdf"));
  await user.keyboard("{Escape}");
  expect(screen.getAllByText("file-1.pdf").length).toBeGreaterThan(0);
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Which sources?",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({ doc_ids: ["d1", "d101"] }),
    );
  });
  expect(
    h.documentsMock.mock.calls.every(([, params]) => params.page_size === 100),
  ).toBe(true);
});

it("keeps selected scope on a failed search and offers retry", async () => {
  h.documentsMock.mockImplementation(async (_dataset, params) => {
    if (params.keywords) throw new Error("directory unavailable");
    return { total: 1, documents: [{ id: "d1", name: "guide.pdf" }] };
  });
  const user = userEvent.setup();
  renderPage();
  const scope = screen.getByRole("combobox", { name: "文档范围" });
  await user.click(scope);
  await user.click(await screen.findByText("guide.pdf"));
  await user.type(scope, "missing");
  await screen.findByText("directory unavailable");
  await user.click(screen.getByRole("button", { name: /重\s*试/ }));
  await user.keyboard("{Escape}");
  expect(screen.getAllByText("guide.pdf").length).toBeGreaterThan(0);
  await user.type(screen.getByRole("textbox", { name: "检索问题" }), "Scope?");
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({ doc_ids: ["d1"] }),
    );
  });
});

it("constructs manual OR conditions with a numeric zero without JSON", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("combobox", { name: "筛选方式" }));
  await user.click(screen.getByText("手动条件"));
  await user.type(screen.getByRole("combobox", { name: "字段 1" }), "version");
  await user.click(screen.getByRole("combobox", { name: "条件关系" }));
  await user.click(screen.getByText("任一满足（OR）"));
  await user.click(screen.getByRole("combobox", { name: "值类型 1" }));
  await user.click(screen.getByText("数字"));
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Revision?",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({
        meta_data_filter: {
          method: "manual",
          logic: "or",
          manual: [{ key: "version", op: "=", value: 0 }],
        },
      }),
    );
  });
});

it.each([
  ["自动推断", { method: "auto" }],
  [
    "限定字段自动推断",
    { method: "semi_auto", semi_auto: [{ key: "category" }] },
  ],
])(
  "submits %s and keeps its model dependency visible",
  async (mode, expected) => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("combobox", { name: "筛选方式" }));
    await user.click(screen.getByText(mode));
    if (mode === "限定字段自动推断") {
      await user.click(screen.getByRole("combobox", { name: "字段 1" }));
      await user.click(
        await screen.findByText("category", {
          selector: ".ant-select-item-option-content",
        }),
      );
    }
    expect(screen.getByText(/对话模型/)).toBeInTheDocument();
    await user.type(
      screen.getByRole("textbox", { name: "检索问题" }),
      "Question",
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await waitFor(() => {
      expect(h.retrievalMock).toHaveBeenCalledWith(
        expect.objectContaining({ meta_data_filter: expected }),
      );
    });
  },
);

it("marks changed drafts and paginates with the tested request", async () => {
  h.data = { ...sampleResult, total: 61 };
  const user = userEvent.setup();
  renderPage();
  const question = screen.getByRole("textbox", { name: "检索问题" });
  await user.type(question, "original question");
  await user.keyboard("{Control>}{Enter}{/Control}");
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledTimes(1);
  });
  await user.clear(question);
  await user.type(question, "changed question");
  expect(screen.getByText(/表单已修改/)).toBeInTheDocument();
  await user.click(screen.getByTitle("2"));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ question: "original question", page: 2 }),
    );
  });
});

it("selects and clears configured rerank full IDs", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("button", { name: /检索策略与模型/ }));
  await user.click(screen.getByRole("combobox", { name: "重排模型" }));
  await user.click(screen.getByText("bge-reranker (API)"));
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Question",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({ rerank_id: "bge-reranker@API" }),
    );
  });
  expect(h.modelsMock).toHaveBeenCalledWith("rerank");
  await user.click(screen.getByLabelText("close-circle"));
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ rerank_id: undefined }),
    );
  });
});

it("hides a service response that violates the selected document scope", async () => {
  h.data = {
    ...sampleResult,
    chunks: [
      {
        ...sampleResult.chunks[0],
        document_id: "outside",
        document_name: "outside.pdf",
      },
    ],
  };
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("combobox", { name: "文档范围" }));
  await user.click(await screen.findByText("guide.pdf"));
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Question",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await screen.findByText(/请求范围之外的来源/);
  expect(screen.queryByText("matched chunk text")).not.toBeInTheDocument();
  expect(screen.queryByText("outside.pdf")).not.toBeInTheDocument();
});

it.each([
  ["0.25", "0.75"],
  ["8", "2"],
])(
  "builds fusion weights %s,%s against the committed service contract",
  async (sparseWeight, denseWeight) => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("button", { name: /检索策略与模型/ }));
    await user.click(screen.getByRole("combobox", { name: "检索模式" }));
    await user.click(screen.getByText("融合检索"));
    expect(screen.getByText(/权重按总和归一化/)).toBeInTheDocument();
    const sparse = screen.getByRole("spinbutton", { name: "融合关键词权重" });
    const dense = screen.getByRole("spinbutton", { name: "融合语义权重" });
    await user.clear(sparse);
    await user.type(sparse, sparseWeight);
    await user.clear(dense);
    await user.type(dense, denseWeight);
    await user.type(
      screen.getByRole("textbox", { name: "检索问题" }),
      "Question",
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await waitFor(() => {
      expect(h.retrievalMock).toHaveBeenCalledWith(
        expect.objectContaining({
          search_mode: {
            type: "fusion",
            weights: `${sparseWeight},${denseWeight}`,
          },
        }),
      );
    });
  },
);

it("opens a source deep link in a new tab while keeping the tested workbench", () => {
  h.data = {
    ...sampleResult,
    chunks: [
      { ...sampleResult.chunks[0], id: "chunk/a?b", document_id: "doc/a" },
    ],
  };
  renderPage();
  const link = screen.getByRole("link", { name: "查看来源切片（新标签页）" });
  expect(link).toHaveAttribute(
    "href",
    "/knowledge/kb1/documents/doc%2Fa/chunks?chunkId=chunk%2Fa%3Fb",
  );
  expect(link).toHaveAttribute("target", "_blank");
  expect(link).toHaveAttribute("rel", "noopener noreferrer");
});

it("blocks unknown semi-auto search text instead of selecting an arbitrary key", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("combobox", { name: "筛选方式" }));
  await user.click(screen.getByText("限定字段自动推断"));
  await user.type(
    screen.getByRole("combobox", { name: "字段 1" }),
    "missing_category",
  );
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Question",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await screen.findByText("筛选草稿无效，请修正后再检索。");
  expect(h.retrievalMock).not.toHaveBeenCalled();
});

it.each([
  { fields: ["missing_category"] },
  { fields: [{ key: "category" }, { key: "missing_category" }] },
])(
  "rejects unknown semi-auto fields from advanced JSON: %j",
  async ({ fields }) => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("button", { name: /高级 JSON/ }));
    const draft = JSON.stringify({ method: "semi_auto", semi_auto: fields });
    const input = screen.getByRole("textbox", { name: "高级 JSON" });
    await user.click(input);
    await user.paste(draft);
    await user.type(
      screen.getByRole("textbox", { name: "检索问题" }),
      "Question",
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await screen.findByText(
      "所选字段不存在于本库元数据中，请选择已有字段后再检索。",
    );
    expect(input).toHaveValue(draft);
    expect(h.retrievalMock).not.toHaveBeenCalled();
  },
);

it("checks semi-auto fields against a fresh directory instead of a stale cached candidate", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("combobox", { name: "筛选方式" }));
  await user.click(screen.getByText("限定字段自动推断"));
  await user.click(screen.getByRole("combobox", { name: "字段 1" }));
  await user.click(
    await screen.findByText("category", {
      selector: ".ant-select-item-option-content",
    }),
  );
  h.metadataKeysMock.mockResolvedValue(["version"]);
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Question",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await screen.findByText(
    "所选字段不存在于本库元数据中，请选择已有字段后再检索。",
  );
  expect(h.metadataKeysMock).toHaveBeenCalledTimes(2);
  expect(h.retrievalMock).not.toHaveBeenCalled();
  expect(
    screen.getByRole("combobox", { name: "字段 1" }).closest(".ant-select"),
  ).toHaveTextContent("category");
});

it.each([
  { result: new Error("directory unavailable") },
  { result: null },
  { result: ["category", null] },
])(
  "does not submit semi-auto filters when field lookup fails or is malformed: %j",
  async ({ result }) => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("combobox", { name: "筛选方式" }));
    await user.click(screen.getByText("限定字段自动推断"));
    await user.click(screen.getByRole("combobox", { name: "字段 1" }));
    await user.click(
      await screen.findByText("category", {
        selector: ".ant-select-item-option-content",
      }),
    );
    if (result instanceof Error) h.metadataKeysMock.mockRejectedValue(result);
    else h.metadataKeysMock.mockResolvedValue(result);
    await user.type(
      screen.getByRole("textbox", { name: "检索问题" }),
      "Question",
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await screen.findByText(/限定字段自动推断需先成功加载元数据字段目录/);
    expect(h.retrievalMock).not.toHaveBeenCalled();
    expect(
      screen.getByRole("combobox", { name: "字段 1" }).closest(".ant-select"),
    ).toHaveTextContent("category");
    h.metadataKeysMock.mockResolvedValue(["category"]);
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await waitFor(() => {
      expect(h.retrievalMock).toHaveBeenCalledWith(
        expect.objectContaining({
          meta_data_filter: {
            method: "semi_auto",
            semi_auto: [{ key: "category" }],
          },
        }),
      );
    });
  },
);

it("keeps cleared numbers numeric, blocks null submission, and accepts a repaired zero", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole("combobox", { name: "筛选方式" }));
  await user.click(screen.getByText("手动条件"));
  await user.type(screen.getByRole("combobox", { name: "字段 1" }), "version");
  await user.click(screen.getByRole("combobox", { name: "值类型 1" }));
  await user.click(screen.getByText("数字"));
  const number = screen.getByRole("spinbutton", { name: "值 1" });
  await user.clear(number);
  expect(screen.getByRole("spinbutton", { name: "值 1" })).toBe(number);
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Question",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await screen.findByText("筛选草稿无效，请修正后再检索。");
  expect(h.retrievalMock).not.toHaveBeenCalled();
  await user.type(number, "0");
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({
        meta_data_filter: {
          method: "manual",
          logic: "and",
          manual: [{ key: "version", op: "=", value: 0 }],
        },
      }),
    );
  });
});

it("does not start retrieval after leaving the page during field validation", async () => {
  const user = userEvent.setup();
  const view = renderPage();
  await user.click(screen.getByRole("combobox", { name: "筛选方式" }));
  await user.click(screen.getByText("限定字段自动推断"));
  await user.click(screen.getByRole("combobox", { name: "字段 1" }));
  await user.click(
    await screen.findByText("category", {
      selector: ".ant-select-item-option-content",
    }),
  );
  let resolveKeys: (keys: string[]) => void = () => {
    throw new Error("lookup not started");
  };
  const lookup = new Promise<string[]>((resolve) => {
    resolveKeys = resolve;
  });
  h.metadataKeysMock.mockReturnValue(lookup);
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Question",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.metadataKeysMock).toHaveBeenCalledTimes(2);
  });
  expect(screen.getByRole("textbox", { name: "检索问题" })).toBeDisabled();
  view.unmount();
  await act(async () => {
    resolveKeys(["category"]);
    await lookup;
  });
  expect(h.retrievalMock).not.toHaveBeenCalled();
});

it.each(["[null]", "[{}]", '[["ops"]]', "[1e309]", "[]", '["ops",false,{}]'])(
  "blocks invalid advanced JSON exclusion lists without dropping the condition: %s",
  async (list) => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("button", { name: /高级 JSON/ }));
    const input = screen.getByRole("textbox", { name: "高级 JSON" });
    const draft = `{"method":"manual","manual":[{"key":"category","op":"not in","value":${list}}]}`;
    await user.click(input);
    await user.paste(draft);
    await user.type(
      screen.getByRole("textbox", { name: "检索问题" }),
      "Question",
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await screen.findByText(
      "列表至少包含一个文本、有限数字或布尔值，不能包含 null、对象、嵌套列表或非有限数字。",
      { selector: ".ant-alert-title" },
    );
    expect(h.retrievalMock).not.toHaveBeenCalled();
    expect(input).toHaveValue(draft);
    expect(screen.getByText("条件 JSON 尚未有效")).toBeInTheDocument();
    await user.clear(input);
    await user.paste(
      '{"method":"manual","manual":[{"key":"category","op":"in","value":["ops","support"]}]}',
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await waitFor(() => {
      expect(h.retrievalMock).toHaveBeenCalledWith(
        expect.objectContaining({
          meta_data_filter: {
            method: "manual",
            logic: "and",
            manual: [{ key: "category", op: "in", value: ["ops", "support"] }],
          },
        }),
      );
    });
  },
);

it.each(["[0]", "[false]", '["ops",0,false]'])(
  "blocks unsafe negative membership without converting the draft: %s",
  async (list) => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("button", { name: /高级 JSON/ }));
    const input = screen.getByRole("textbox", { name: "高级 JSON" });
    const draft = `{"method":"manual","manual":[{"key":"category","op":"not in","value":${list}}]}`;
    await user.click(input);
    await user.paste(draft);
    await user.type(
      screen.getByRole("textbox", { name: "检索问题" }),
      "Question",
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await screen.findByText(/负向筛选.*暂不可用/, {
      selector: ".ant-alert-title",
    });
    expect(h.retrievalMock).not.toHaveBeenCalled();
    expect(input).toHaveValue(draft);
  },
);

it("keeps typed tags intact and submits positive membership with original types", async () => {
  const user = userEvent.setup();
  const view = renderPage();
  await user.click(screen.getByRole("button", { name: /高级 JSON/ }));
  const input = screen.getByRole("textbox", { name: "高级 JSON" });
  await user.click(input);
  await user.paste(
    '{"method":"manual","manual":[{"key":"category","op":"in","value":["ops",0,false]}]}',
  );
  const removeTag = async (label: string) => {
    const tag = view.container.querySelector(
      `.ant-select-selection-item[title="${label}"]`,
    );
    expect(tag).not.toBeNull();
    await user.click(tag!.querySelector(".ant-select-selection-item-remove")!);
  };
  await removeTag("ops");
  expect(
    JSON.parse((input as HTMLTextAreaElement).value).manual[0].value,
  ).toEqual([0, false]);
  await user.type(
    screen.getByRole("textbox", { name: "检索问题" }),
    "Question",
  );
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({
        meta_data_filter: {
          method: "manual",
          logic: "and",
          manual: [{ key: "category", op: "in", value: [0, false] }],
        },
      }),
    );
  });
  await removeTag("0");
  await removeTag("false");
  expect(
    JSON.parse((input as HTMLTextAreaElement).value).manual[0].value,
  ).toEqual([]);
  h.retrievalMock.mockClear();
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  expect(h.retrievalMock).not.toHaveBeenCalled();
  await user.type(screen.getByRole("combobox", { name: "值 1" }), "ops{Enter}");
  await user.click(screen.getByRole("button", { name: /检索测试/ }));
  await waitFor(() => {
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({
        meta_data_filter: {
          method: "manual",
          logic: "and",
          manual: [{ key: "category", op: "in", value: ["ops"] }],
        },
      }),
    );
  });
});

it.each(["and", "or"])(
  "blocks text not in in a complete %s draft, disables its builder option and preserves repairability",
  async (logic) => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("button", { name: /高级 JSON/ }));
    const input = screen.getByRole("textbox", { name: "高级 JSON" });
    const draft = JSON.stringify({
      method: "manual",
      logic,
      manual: [
        { key: "author", op: "not in", value: ["Alice"] },
        { key: "category", op: "in", value: ["ops"] },
      ],
    });
    await user.click(input);
    await user.paste(draft);
    await user.type(
      screen.getByRole("textbox", { name: "检索问题" }),
      "Question",
    );
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await screen.findByText(/多值文档即使包含被排除值仍可能命中/, {
      selector: ".ant-alert-title",
    });
    expect(h.retrievalMock).not.toHaveBeenCalled();
    expect(input).toHaveValue(draft);
    expect(screen.getByText("条件 JSON 尚未有效")).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: "比较方式 1" }));
    const option = screen
      .getAllByText("不属于列表（暂不可用）")
      .find((label) => label.closest(".ant-select-item-option"));
    expect(option?.closest(".ant-select-item-option")).toHaveClass(
      "ant-select-item-option-disabled",
    );
    for (const label of ["不等于（暂不可用）", "不包含（暂不可用）"]) {
      expect(
        screen.getByText(label).closest(".ant-select-item-option"),
      ).toHaveClass("ant-select-item-option-disabled");
    }
    await user.click(screen.getByRole("combobox", { name: "比较方式 1" }));
    await user.clear(input);
    await user.paste(draft.replace('"not in"', '"in"'));
    await user.click(screen.getByRole("button", { name: /检索测试/ }));
    await waitFor(() => {
      expect(h.retrievalMock).toHaveBeenCalledWith(
        expect.objectContaining({
          meta_data_filter: {
            method: "manual",
            logic,
            manual: [
              { key: "author", op: "in", value: ["Alice"] },
              { key: "category", op: "in", value: ["ops"] },
            ],
          },
        }),
      );
    });
  },
);

it.each(["自动推断", "限定字段自动推断"])(
  "discloses generated exclusion limits for %s without disabling the mode",
  async (mode) => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("combobox", { name: "筛选方式" }));
    await user.click(screen.getByText(mode));
    expect(screen.getByText(/本页无法校验内部生成的条件/)).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "筛选方式" })).toBeEnabled();
  },
);
