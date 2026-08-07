import { describe, test, expect, mock, beforeEach, afterEach } from "bun:test";
import * as realConfig from "../../src/config";

function setupConfigMock() {
  mock.module("../../src/config", () => ({
    ...realConfig,
    getActiveProfile: mock(() =>
      Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
    ),
  }));
}

function setupFetchMock(responseData: unknown) {
  const fetchMock = mock(() =>
    Promise.resolve({ success: true, data: responseData })
  );
  mock.module("ofetch", () => ({
    ofetch: fetchMock,
    FetchError: class FetchError extends Error {},
  }));
  return fetchMock;
}

const sampleTool = {
  id: 1,
  name: "calculator",
  title: "Calculator",
  description: "A simple calculator",
  isDefault: false,
  createdAt: "2026-01-01T00:00:00.000Z",
  updatedAt: "2026-01-01T00:00:00.000Z",
};

describe("tool client", () => {
  beforeEach(setupConfigMock);
  afterEach(() => mock.restore());

  test("listTools calls GET /api/v1/admin/tools", async () => {
    const fetchMock = setupFetchMock([sampleTool]);

    const { listTools } = await import(
      `../../src/client/tool.ts?t=${Date.now()}`
    );
    const result = await listTools();

    expect(result).toHaveLength(1);
    expect(result[0].name).toBe("calculator");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/tools");
    expect(calls[0][1].method).toBe("GET");
  });

  test("getTool calls GET /api/v1/admin/tools/:name", async () => {
    const fetchMock = setupFetchMock(sampleTool);

    const { getTool } = await import(
      `../../src/client/tool.ts?t=${Date.now()}`
    );
    const result = await getTool("calculator");

    expect(result.name).toBe("calculator");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/tools/calculator");
    expect(calls[0][1].method).toBe("GET");
  });

  test("createTool calls POST /api/v1/admin/tools with body", async () => {
    const body = { name: "calculator", title: "Calculator" };
    const fetchMock = setupFetchMock(sampleTool);

    const { createTool } = await import(
      `../../src/client/tool.ts?t=${Date.now()}`
    );
    const result = await createTool(body);

    expect(result.title).toBe("Calculator");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/tools");
    expect(calls[0][1].method).toBe("POST");
    expect(calls[0][1].body).toEqual(body);
  });

  test("updateTool calls PUT /api/v1/admin/tools/:name", async () => {
    const body = { title: "Updated Calculator" };
    const fetchMock = setupFetchMock({ ...sampleTool, title: "Updated Calculator" });

    const { updateTool } = await import(
      `../../src/client/tool.ts?t=${Date.now()}`
    );
    const result = await updateTool("calculator", body);

    expect(result.title).toBe("Updated Calculator");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/tools/calculator");
    expect(calls[0][1].method).toBe("PUT");
    expect(calls[0][1].body).toEqual(body);
  });

  test("deleteTool calls DELETE /api/v1/admin/tools/:name", async () => {
    const fetchMock = setupFetchMock({ success: true });

    const { deleteTool } = await import(
      `../../src/client/tool.ts?t=${Date.now()}`
    );
    await deleteTool("calculator");

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/tools/calculator");
    expect(calls[0][1].method).toBe("DELETE");
  });

  test("encodes special characters in tool name", async () => {
    const fetchMock = setupFetchMock(sampleTool);

    const { getTool } = await import(
      `../../src/client/tool.ts?t=${Date.now()}`
    );
    await getTool("tool name");

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/tools/tool%20name");
  });
});
