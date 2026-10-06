import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import KnowledgeIndexCard from "./KnowledgeIndexCard";

const h = vi.hoisted(() => ({
  data: {} as Record<string, unknown> | undefined,
  error: undefined as unknown,
  refetch: vi.fn(),
  mutate: vi.fn(),
  writable: true,
}));
vi.mock("@/queries/useKnowledgeManagement", () => ({
  useKnowledgeResource: () => ({
    data: h.data,
    error: h.error,
    isLoading: false,
    isFetching: false,
    refetch: h.refetch,
  }),
  useKnowledgeAction: () => ({ mutateAsync: h.mutate, isPending: false }),
}));
vi.mock("@/hooks/useCanWrite", () => ({ useCanWrite: () => h.writable }));

beforeEach(() => {
  h.data = {
    id: "t1",
    progress: 0.4,
    progress_msg: "Extracting relationships",
  };
  h.error = undefined;
  h.writable = true;
  h.refetch.mockReset().mockImplementation(async () => ({ data: h.data }));
  h.mutate.mockReset().mockResolvedValue({
    task_id: "t1",
    request_accepted: true,
    cancel_requested: true,
  });
});

async function confirmCancel() {
  fireEvent.click(screen.getByRole("button", { name: "请求取消" }));
  await waitFor(() => {
    expect(screen.getAllByRole("button", { name: "请求取消" })).toHaveLength(2);
  });
  fireEvent.click(screen.getAllByRole("button", { name: "请求取消" })[1]);
}

it("cancels the current dataset/type task with its traced ID and reads state back without claiming worker exit", async () => {
  h.mutate.mockImplementation(async () => {
    h.data = { id: "t1", progress: -1, progress_msg: "[cancel_requested]" };
    return {
      task_id: "t1",
      request_accepted: true,
      cancel_requested: true,
      task: h.data,
    };
  });
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  expect(screen.getByRole("button", { name: "运行索引" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "删除索引产物" })).toBeDisabled();
  await confirmCancel();
  await waitFor(() => {
    expect(h.mutate).toHaveBeenCalledWith({
      method: "post",
      resource: "index/cancel",
      params: { type: "graph" },
      data: { task_id: "t1" },
    });
  });
  await waitFor(() => {
    expect(h.refetch).toHaveBeenCalledTimes(2);
  });
  expect(screen.getByText(/worker 退出尚未确认/)).toBeInTheDocument();
  expect(screen.queryByText("已取消")).not.toBeInTheDocument();
});

it("refuses stale task confirmations and performs no mutation", async () => {
  h.refetch.mockResolvedValue({ data: { id: "replacement", progress: 0.3 } });
  render(<KnowledgeIndexCard id="kb1" kind="raptor" />);
  await confirmCancel();
  await waitFor(() =>
    expect(
      screen.getByText("任务状态读取失败，请刷新后再操作。"),
    ).toBeInTheDocument(),
  );
  expect(h.mutate).not.toHaveBeenCalled();
});

it("pins the task ID when confirmation opens even if polling replaces the task before confirmation", async () => {
  const view = render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  fireEvent.click(screen.getByRole("button", { name: "请求取消" }));
  await waitFor(() => {
    expect(screen.getAllByRole("button", { name: "请求取消" })).toHaveLength(2);
  });
  h.data = { id: "replacement", progress: 0.3 };
  view.rerender(<KnowledgeIndexCard id="kb1" kind="graph" />);
  fireEvent.click(screen.getAllByRole("button", { name: "请求取消" })[1]);
  expect(
    await screen.findByText("任务状态读取失败，请刷新后再操作。"),
  ).toBeInTheDocument();
  expect(h.mutate).not.toHaveBeenCalled();
});

it("keeps a cancellation failure visible and does not claim acceptance", async () => {
  h.mutate.mockRejectedValue(new Error("Cancellation binding changed"));
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  await confirmCancel();
  await waitFor(() =>
    expect(
      screen.getByText("Cancellation binding changed"),
    ).toBeInTheDocument(),
  );
  expect(screen.queryByText(/取消已受理/)).not.toBeInTheDocument();
});

it("shows read failure rather than no index and disables writes", () => {
  h.data = undefined;
  h.error = new Error("No authorization");
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  expect(screen.getByText("No authorization")).toBeInTheDocument();
  expect(screen.queryByText("尚未建立索引任务")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "运行索引" })).toBeDisabled();
});

it("explains that Mindmap cleanup only unbinds the task and requires an empty readback", async () => {
  h.data = { id: "m1", progress: 1 };
  h.mutate.mockImplementation(async () => {
    h.data = {};
    return {};
  });
  render(<KnowledgeIndexCard id="kb1" kind="mindmap" />);
  fireEvent.click(screen.getByRole("button", { name: "解除任务绑定" }));
  expect(
    await screen.findByText(
      "当前接口只解除 Mindmap 任务绑定，不会删除已有思维导图数据。",
    ),
  ).toBeInTheDocument();
  fireEvent.click(screen.getAllByRole("button", { name: "解除任务绑定" })[1]);
  await waitFor(() => {
    expect(h.mutate).toHaveBeenCalledWith({
      method: "delete",
      resource: "index",
      params: { type: "mindmap" },
    });
  });
  expect(await screen.findByText("尚未建立索引任务")).toBeInTheDocument();
});

it("hides write controls for a read-only role", () => {
  h.writable = false;
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  expect(
    screen.queryByRole("button", { name: "请求取消" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "删除索引产物" }),
  ).not.toBeInTheDocument();
});

it("keeps an accepted cancellation with failed readback visibly unconfirmed", async () => {
  h.refetch
    .mockResolvedValueOnce({ data: h.data })
    .mockResolvedValueOnce({ error: new Error("Readback offline") });
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  await confirmCancel();
  expect(
    await screen.findByText("操作已受理，但状态回读失败，请刷新确认。"),
  ).toBeInTheDocument();
  expect(screen.queryByText(/取消已受理/)).not.toBeInTheDocument();
});

it("does not confirm a newly accepted run when the traced task remains absent", async () => {
  h.data = {};
  h.mutate.mockResolvedValue({ task_id: "new-task" });
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  fireEvent.click(screen.getByRole("button", { name: "运行索引" }));
  expect(
    await screen.findByText("操作已受理，但状态回读失败，请刷新确认。"),
  ).toBeInTheDocument();
  expect(h.mutate).toHaveBeenCalledWith({
    method: "post",
    resource: "index",
    params: { type: "graph" },
  });
});

it.each([
  { progress: 1, message: "Completed naturally", label: "已完成" },
  { progress: -1, message: "[ERROR] Provider failed", label: "失败" },
])(
  "prioritizes the real terminal state when cancellation races with $message",
  async ({ progress, message, label }) => {
    h.mutate.mockImplementation(async () => {
      h.data = { id: "t1", progress, progress_msg: message };
      return {
        task_id: "t1",
        request_accepted: true,
        cancel_requested: false,
        task: h.data,
      };
    });
    render(<KnowledgeIndexCard id="kb1" kind="graph" />);
    await confirmCancel();
    await waitFor(() => {
      expect(h.refetch).toHaveBeenCalledTimes(2);
    });
    expect(screen.getByText(label)).toBeInTheDocument();
    expect(screen.queryByText(/停止请求已受理/)).not.toBeInTheDocument();
    expect(screen.queryByText(/取消已请求/)).not.toBeInTheDocument();
  },
);

it("shows request acceptance separately while running, then removes that notice when polling reaches a terminal state", async () => {
  h.mutate.mockResolvedValue({
    task_id: "t1",
    request_accepted: true,
    cancel_requested: false,
    task: h.data,
  });
  const view = render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  await confirmCancel();
  expect(
    await screen.findByText(
      "停止请求已受理；当前任务尚未结束，取消是否生效仍需确认。",
    ),
  ).toBeInTheDocument();
  expect(screen.queryByText(/取消已请求/)).not.toBeInTheDocument();
  h.data = { id: "t1", progress: 1, progress_msg: "Completed naturally" };
  view.rerender(<KnowledgeIndexCard id="kb1" kind="graph" />);
  expect(screen.getByText("已完成")).toBeInTheDocument();
  expect(screen.queryByText(/停止请求已受理/)).not.toBeInTheDocument();
});

it("does not use cancel_requested as a substitute for request_accepted", async () => {
  h.mutate.mockResolvedValue({
    task_id: "t1",
    cancel_requested: true,
    task: h.data,
  });
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  await confirmCancel();
  expect(
    await screen.findByText("停止请求未被确认受理，请刷新任务状态后重试。"),
  ).toBeInTheDocument();
  expect(screen.queryByText(/停止请求已受理/)).not.toBeInTheDocument();
});

it("does not mistake an old cancellation marker for a newly cancelled task after a natural failure", async () => {
  h.data = {
    id: "t1",
    progress: 0.4,
    progress_msg: "[cancel_requested] historical attempt",
  };
  h.mutate.mockImplementation(async () => {
    h.data = {
      id: "t1",
      progress: -1,
      progress_msg:
        "[cancel_requested] historical attempt\n[ERROR] Natural failure",
    };
    return {
      task_id: "t1",
      request_accepted: true,
      cancel_requested: false,
      task: h.data,
    };
  });
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  await confirmCancel();
  await waitFor(() => {
    expect(h.refetch).toHaveBeenCalledTimes(2);
  });
  expect(screen.getByText("失败")).toBeInTheDocument();
  expect(screen.queryByText("取消已请求")).not.toBeInTheDocument();
  expect(screen.queryByText(/worker 退出尚未确认/)).not.toBeInTheDocument();
});

it("recognizes a new cancellation marker even when the task already contained a historical marker", async () => {
  h.data = {
    id: "t1",
    progress: 0.4,
    progress_msg: "[cancel_requested] historical attempt",
  };
  h.mutate.mockImplementation(async () => {
    h.data = {
      id: "t1",
      progress: -1,
      progress_msg:
        "[cancel_requested] historical attempt\n[cancel_requested] current request",
    };
    return {
      task_id: "t1",
      request_accepted: true,
      cancel_requested: true,
      task: h.data,
    };
  });
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  await confirmCancel();
  expect(await screen.findByText("取消已请求")).toBeInTheDocument();
  expect(screen.queryByText("失败")).not.toBeInTheDocument();
});

it("does not treat a historical marker as confirmation after the index card remounts", () => {
  h.data = {
    id: "t1",
    progress: -1,
    progress_msg:
      "[cancel_requested] historical attempt\n[ERROR] Natural failure",
  };
  const view = render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  expect(screen.getByText("已结束（取消状态未确认）")).toBeInTheDocument();
  view.unmount();
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  expect(screen.getByText("已结束（取消状态未确认）")).toBeInTheDocument();
  expect(screen.queryByText("取消已请求")).not.toBeInTheDocument();
  expect(screen.queryByText(/worker 退出尚未确认/)).not.toBeInTheDocument();
});

it("requires fresh cancellation evidence again after a locally confirmed card is remounted", async () => {
  h.mutate.mockImplementation(async () => {
    h.data = {
      id: "t1",
      progress: -1,
      progress_msg: "[cancel_requested] current request",
    };
    return {
      task_id: "t1",
      request_accepted: true,
      cancel_requested: true,
      task: h.data,
    };
  });
  const view = render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  await confirmCancel();
  expect(await screen.findByText("取消已请求")).toBeInTheDocument();
  view.unmount();
  render(<KnowledgeIndexCard id="kb1" kind="graph" />);
  expect(screen.getByText("已结束（取消状态未确认）")).toBeInTheDocument();
  expect(screen.queryByText("取消已请求")).not.toBeInTheDocument();
});
