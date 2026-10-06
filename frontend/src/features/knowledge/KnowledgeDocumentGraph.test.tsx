import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { ConfigProvider } from "antd";
import KnowledgeDocumentGraph from "./KnowledgeDocumentGraph";

const h = vi.hoisted(() => ({
  request: vi.fn(),
  data: undefined as unknown,
  error: undefined as unknown,
}));
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeResource: (...args: unknown[]) => {
    h.request(...args);
    return {
      data: h.data,
      error: h.error,
      isLoading: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
}));
beforeEach(() => {
  h.request.mockReset();
  h.data = undefined;
  h.error = undefined;
});
it("reads the selected document graph and shows entities and relationships", () => {
  h.data = {
    graph: {
      nodes: [
        {
          id: "Product v2",
          entity_type: "PRODUCT",
          description: "Second version",
        },
      ],
      edges: [
        { source: "Product v2", target: "Warranty", description: "Two years" },
      ],
    },
  };
  render(
    <ConfigProvider>
      <KnowledgeDocumentGraph
        datasetId="kb1"
        documentId="d1"
        name="Guide"
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  );
  expect(h.request).toHaveBeenCalledWith("kb1", "graph", { doc_id: "d1" });
  expect(screen.getByText("Second version")).toBeInTheDocument();
  expect(screen.getByText("Two years")).toBeInTheDocument();
});
it("reports permission failure instead of presenting it as an empty graph", () => {
  h.error = new Error("No authorization");
  render(
    <ConfigProvider>
      <KnowledgeDocumentGraph
        datasetId="kb1"
        documentId="d1"
        name="Guide"
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  );
  expect(screen.getByText("No authorization")).toBeInTheDocument();
  expect(screen.queryByText("当前文档尚无图谱数据")).not.toBeInTheDocument();
  expect(screen.queryByText("实体 0 个")).not.toBeInTheDocument();
});

it("keeps incident relationships while filtering and supports neighbour navigation", () => {
  h.data = {
    graph: {
      nodes: [
        { id: "Product", description: "Product detail" },
        { id: "Warranty", description: "Warranty detail" },
      ],
      edges: [
        { source: "Product", target: "Warranty", description: "Two years" },
      ],
    },
  };
  render(
    <KnowledgeDocumentGraph
      datasetId="kb1"
      documentId="d1"
      name="Guide"
      onClose={vi.fn()}
    />,
  );
  fireEvent.change(
    screen.getByRole("searchbox", { name: "搜索实体名称、类型或说明" }),
    { target: { value: "Product" } },
  );
  expect(screen.getByText("Two years")).toBeInTheDocument();
  fireEvent.click(
    screen.getAllByRole("button", { name: "查看实体 Product" })[0],
  );
  const dialogs = screen.getAllByRole("dialog");
  const detail = within(dialogs[dialogs.length - 1]);
  expect(detail.getByText("相邻实体与关系")).toBeInTheDocument();
  fireEvent.click(detail.getByRole("button", { name: "查看实体 Warranty" }));
  expect(detail.getByText("Warranty detail")).toBeInTheDocument();
});

it("shows orphan relationships and mindmap-only payloads instead of claiming an empty graph", () => {
  h.data = {
    graph: {
      nodes: [],
      edges: [
        {
          src_id: "External",
          tgt_id: "Target",
          description: "Existing relation",
        },
      ],
    },
    mind_map: { name: "Topics", children: [{ name: "Installation" }] },
  };
  render(
    <KnowledgeDocumentGraph
      datasetId="kb1"
      documentId="d1"
      name="Guide"
      onClose={vi.fn()}
    />,
  );
  expect(screen.getByText("Existing relation")).toBeInTheDocument();
  expect(screen.getByText("Installation")).toBeInTheDocument();
  expect(screen.queryByText("当前文档尚无图谱数据")).not.toBeInTheDocument();
});
