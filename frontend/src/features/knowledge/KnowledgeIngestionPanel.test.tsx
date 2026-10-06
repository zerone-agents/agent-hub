import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import KnowledgeIngestionPanel from "./KnowledgeIngestionPanel";

const h = vi.hoisted(() => ({
  request: vi.fn(),
  logs: [] as Record<string, unknown>[],
  total: 0,
  error: undefined as unknown,
  detail: undefined as unknown,
  fetching: false,
}));
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeResource: (id: string, resource: string) => {
    h.request(id, resource);
    const list = resource.startsWith("ingestions?");
    return {
      data: list
        ? { logs: h.logs, total: h.total }
        : resource === "ingestions/summary"
          ? {
              doc_num: 2,
              chunk_num: 3,
              token_num: 40,
              status: { running_count: 1 },
            }
          : id && resource.startsWith("ingestions/")
            ? h.detail
            : undefined,
      error: list ? h.error : undefined,
      isLoading: false,
      isFetching: h.fetching,
      refetch: vi.fn(),
    };
  },
  useKnowledgeAction: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));
vi.mock("@/hooks/useCanWrite", () => ({ useCanWrite: () => false }));

beforeEach(() => {
  h.request.mockReset();
  h.logs = [];
  h.total = 0;
  h.error = undefined;
  h.detail = undefined;
  h.fetching = false;
});

function latestListQuery() {
  const calls = h.request.mock.calls.filter(([, resource]) =>
    String(resource).startsWith("ingestions?"),
  );
  return new URLSearchParams(String(calls.at(-1)?.[1]).split("?")[1]);
}

it("starts with file logs and switches to dataset logs without losing the dataset scope", () => {
  render(<KnowledgeIngestionPanel id="kb1" />);
  expect(latestListQuery().get("log_type")).toBe("file");
  fireEvent.click(screen.getByRole("tab", { name: "库级任务日志" }));
  expect(latestListQuery().get("log_type")).toBe("dataset");
  expect(latestListQuery().get("page")).toBe("1");
  expect(h.request.mock.calls.every(([id]) => id === "kb1" || id === "")).toBe(
    true,
  );
});

it("searches names on the server and resets pagination after a filter change", async () => {
  h.total = 31;
  render(<KnowledgeIngestionPanel id="kb1" />);
  fireEvent.click(screen.getByTitle("2"));
  expect(latestListQuery().get("page")).toBe("2");
  const search = screen.getByRole("searchbox", { name: "搜索文件或任务名称" });
  fireEvent.change(search, { target: { value: "failure report" } });
  fireEvent.keyDown(search, { key: "Enter", code: "Enter" });
  await waitFor(() => {
    expect(latestListQuery().get("keywords")).toBe("failure report");
  });
  expect(latestListQuery().get("page")).toBe("1");
});

it("retains the file document association when the detail endpoint omits file fields", async () => {
  h.logs = [
    {
      id: "log/1",
      kb_id: "kb1",
      document_id: "d/1",
      document_name: "Guide.pdf",
      task_type: "Parse",
      operation_status: "4",
      progress_msg: "Provider timed out",
    },
  ];
  h.detail = {
    id: "log/1",
    operation_status: "4",
    progress_msg: "Provider timed out",
  };
  render(<KnowledgeIngestionPanel id="kb1" />);
  expect(screen.getByRole("link", { name: "查看文档切片" })).toHaveAttribute(
    "href",
    "/knowledge/kb1/documents/d%2F1/chunks",
  );
  fireEvent.click(screen.getByRole("button", { name: "查看详情" }));
  await waitFor(() => {
    expect(h.request).toHaveBeenCalledWith("kb1", "ingestions/log%2F1");
  });
  expect(screen.getAllByRole("link", { name: "查看文档切片" })).toHaveLength(2);
  expect(screen.getAllByText("Provider timed out")).toHaveLength(2);
});

it("does not treat a failed list request as empty logs", () => {
  h.error = new Error("No authorization");
  render(<KnowledgeIngestionPanel id="kb1" />);
  expect(screen.getByText("No authorization")).toBeInTheDocument();
  expect(screen.queryByText("当前条件下没有日志")).not.toBeInTheDocument();
});

it("preserves the recorded failure status and only annotates a historical cancellation marker", () => {
  h.logs = [
    {
      id: "cancel-log",
      kb_id: "kb1",
      task_type: "GraphRAG",
      operation_status: "4",
      progress: -1,
      progress_msg: "[cancel_requested] Task stopped by user",
    },
  ];
  render(<KnowledgeIngestionPanel id="kb1" />);
  expect(screen.getByText("失败")).toBeInTheDocument();
  expect(screen.getByText("含取消请求标记")).toBeInTheDocument();
  expect(screen.queryByText("取消已请求")).not.toBeInTheDocument();
});

it("does not classify progress -2 with a cancellation marker as cancellation", () => {
  h.logs = [
    {
      id: "negative-log",
      kb_id: "kb1",
      task_type: "GraphRAG",
      operation_status: "4",
      progress: -2,
      progress_msg: "[cancel_requested] historical attempt\n[ERROR] failed",
    },
  ];
  render(<KnowledgeIngestionPanel id="kb1" />);
  expect(screen.getByText("失败")).toBeInTheDocument();
  expect(screen.queryByText("取消已请求")).not.toBeInTheDocument();
  expect(screen.queryByText("含取消请求标记")).not.toBeInTheDocument();
});

it("does not replace a natural failure after a historical cancellation marker in either log list or detail", async () => {
  const log = {
    id: "historical-log",
    kb_id: "kb1",
    task_type: "GraphRAG",
    operation_status: "4",
    progress: -1,
    progress_msg:
      "[cancel_requested] historical attempt\n[ERROR] Provider failed naturally",
  };
  h.logs = [log];
  h.detail = log;
  render(<KnowledgeIngestionPanel id="kb1" />);
  expect(screen.getByText("失败")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "查看详情" }));
  await waitFor(() => {
    expect(h.request).toHaveBeenCalledWith("kb1", "ingestions/historical-log");
  });
  expect(screen.getAllByText("失败")).toHaveLength(2);
  expect(screen.getAllByText("含取消请求标记")).toHaveLength(2);
  expect(screen.queryByText("取消已请求")).not.toBeInTheDocument();
});

it("blocks foreign document rows and discards selected details when switching datasets", () => {
  h.logs = [
    {
      id: "foreign",
      kb_id: "other",
      document_id: "secret",
      document_name: "Other.pdf",
    },
  ];
  const view = render(<KnowledgeIngestionPanel id="kb1" />);
  expect(
    screen.queryByRole("link", { name: "查看文档切片" }),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "查看详情" })).toBeDisabled();
  h.logs = [
    { id: "own", kb_id: "kb1", document_id: "d1", document_name: "Own.pdf" },
  ];
  view.rerender(<KnowledgeIngestionPanel id="kb1" />);
  fireEvent.click(screen.getByRole("button", { name: "查看详情" }));
  view.rerender(<KnowledgeIngestionPanel id="kb2" />);
  expect(h.request.mock.calls.at(-1)?.[0]).toBe("");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
