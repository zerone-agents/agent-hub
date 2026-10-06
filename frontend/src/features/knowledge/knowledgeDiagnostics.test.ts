import { expect, it } from "vitest";
import {
  getIndexState,
  graphEndpoint,
  ingestionLogResource,
  logDocumentId,
} from "./knowledgeDiagnostics";

it("serializes REST repeated filters without brackets and preserves dates, pagination and type", () => {
  const resource = ingestionLogResource({
    page: 3,
    logType: "file",
    keywords: " report + 安全 ",
    status: ["1", "4"],
    from: "2026-10-04T01:00:00.000Z",
    to: "2026-10-05T02:00:00.000Z",
    desc: false,
  });
  const params = new URLSearchParams(resource.split("?")[1]);
  expect(params.getAll("operation_status")).toEqual(["1", "4"]);
  expect(params.has("operation_status[]")).toBe(false);
  expect(params.get("log_type")).toBe("file");
  expect(params.get("page")).toBe("3");
  expect(params.get("page_size")).toBe("15");
  expect(params.get("keywords")).toBe("report + 安全");
  expect(params.get("create_date_from")).toBe("2026-10-04T01:00:00.000Z");
  expect(params.get("create_date_to")).toBe("2026-10-05T02:00:00.000Z");
  expect(params.get("desc")).toBe("false");
});

it("distinguishes unconfirmed, absent, queued, active, failed and cancellation-requested task records", () => {
  expect(getIndexState(undefined)).toBe("unknown");
  expect(getIndexState({})).toBe("empty");
  expect(getIndexState({ id: "t1", progress: 0 })).toBe("queued");
  expect(getIndexState({ id: "t1", progress: 0.5 })).toBe("running");
  expect(getIndexState({ id: "t1", progress: 1 })).toBe("done");
  expect(
    getIndexState({
      id: "t1",
      progress: -1,
      progress_msg: "[ERROR] provider failed",
    }),
  ).toBe("failed");
  expect(
    getIndexState({
      id: "t1",
      progress: -1,
      progress_msg: "[cancel_requested] Task stopped by user",
    }),
  ).toBe("endedUnconfirmed");
  expect(
    getIndexState(
      { id: "t1", progress: -1, progress_msg: "[cancel_requested]" },
      { taskId: "t1", markerCount: 0 },
    ),
  ).toBe("cancelRequested");
  expect(
    getIndexState(
      {
        id: "t1",
        progress: -1,
        progress_msg: "[cancel_requested] historical\n[ERROR] failed",
      },
      { taskId: "t1", markerCount: 1 },
    ),
  ).toBe("failed");
  expect(getIndexState({ id: "t1", progress: "1" })).toBe("unknown");
  expect(getIndexState({ id: {}, progress: 0.5 })).toBe("unknown");
  expect(
    getIndexState({
      id: "t1",
      progress: -2,
      progress_msg: "[cancel_requested]",
    }),
  ).toBe("failed");
});

it("keeps synthetic dataset tasks out of document navigation and supports graph endpoint aliases", () => {
  expect(logDocumentId({ document_id: "graph_raptor_x" })).toBeUndefined();
  expect(logDocumentId({ document_id: "dataflow_x" })).toBeUndefined();
  expect(logDocumentId({ document_id: "d1" })).toBe("d1");
  expect(graphEndpoint({ src_id: "A" }, "source")).toBe("A");
  expect(graphEndpoint({ target: { id: "B" } }, "target")).toBe("B");
});
