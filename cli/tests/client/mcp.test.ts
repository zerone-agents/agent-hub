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

const sampleMcp = {
  id: 1,
  name: "filesystem",
  title: "Filesystem",
  description: "Access the local filesystem",
  transportType: "sse",
  url: "http://localhost:3001/sse",
  hasHeaders: true,
  isBuiltin: false,
  retryMaxRetries: 3,
  retryTimeoutMs: 5000,
  createdAt: "2026-01-01T00:00:00.000Z",
  updatedAt: "2026-01-01T00:00:00.000Z",
};

const sampleMcpDetail: typeof sampleMcp & {
  headers: Record<string, string>;
} = {
  ...sampleMcp,
  headers: {
    Authorization: "secret-token",
    "X-Api-Key": "sk-leaked",
  },
};

describe("mcp client", () => {
  beforeEach(setupConfigMock);
  afterEach(() => mock.restore());

  test("listMcps calls GET /api/v1/admin/mcps", async () => {
    const fetchMock = setupFetchMock([sampleMcp]);

    const { listMcps } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    const result = await listMcps();

    expect(result).toHaveLength(1);
    expect(result[0].name).toBe("filesystem");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps");
    expect(calls[0][1].method).toBe("GET");
  });

  test("getMcp masks headers and does not leak original secret", async () => {
    const fetchMock = setupFetchMock({
      ...sampleMcpDetail,
      headers: {
        Authorization: "secret-token",
        "X-Api-Key": "sk-leaked",
        "X-Empty": "",
      },
    });

    const { getMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    const result = await getMcp("filesystem");

    const serialized = JSON.stringify(result);
    expect(serialized).not.toContain("secret-token");
    expect(serialized).not.toContain("sk-leaked");
    expect(result.headers.Authorization).toBe("<hidden>");
    expect(result.headers["X-Api-Key"]).toBe("<hidden>");
    expect(result.headers["X-Empty"]).toBe("");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/filesystem");
    expect(calls[0][1].method).toBe("GET");
  });

  test("getMcp handles empty headers", async () => {
    const fetchMock = setupFetchMock({ ...sampleMcpDetail, headers: {} });

    const { getMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    const result = await getMcp("filesystem");

    expect(result.headers).toEqual({});
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/filesystem");
    expect(calls[0][1].method).toBe("GET");
  });

  test("createMcp calls POST /api/v1/admin/mcps with body", async () => {
    const body = {
      name: "filesystem",
      title: "Filesystem",
      description: "Access the local filesystem",
      transportType: "sse" as const,
      url: "http://localhost:3001/sse",
      retryMaxRetries: 3,
      retryTimeoutMs: 5000,
    };
    const fetchMock = setupFetchMock(sampleMcp);

    const { createMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    const result = await createMcp(body);

    expect(result.name).toBe("filesystem");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps");
    expect(calls[0][1].method).toBe("POST");
    expect(calls[0][1].body).toEqual(body);
  });

  test("updateMcp calls PUT /api/v1/admin/mcps/:name", async () => {
    const body = { title: "Updated Filesystem" };
    const fetchMock = setupFetchMock({ ...sampleMcp, title: "Updated Filesystem" });

    const { updateMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    const result = await updateMcp("filesystem", body);

    expect(result.title).toBe("Updated Filesystem");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/filesystem");
    expect(calls[0][1].method).toBe("PUT");
    expect(calls[0][1].body).toEqual(body);
  });

  test("deleteMcp calls DELETE /api/v1/admin/mcps/:name", async () => {
    const fetchMock = setupFetchMock({ success: true });

    const { deleteMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    await deleteMcp("filesystem");

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/filesystem");
    expect(calls[0][1].method).toBe("DELETE");
  });

  test("getMcp encodes special characters in mcp name", async () => {
    const fetchMock = setupFetchMock(sampleMcpDetail);

    const { getMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    await getMcp("mcp name");

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/mcp%20name");
    expect(calls[0][1].method).toBe("GET");
  });

  test("updateMcp encodes special characters in mcp name", async () => {
    const fetchMock = setupFetchMock({
      ...sampleMcp,
      title: "Updated Filesystem",
    });

    const { updateMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    await updateMcp("mcp name", { title: "Updated Filesystem" });

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/mcp%20name");
    expect(calls[0][1].method).toBe("PUT");
  });

  test("deleteMcp encodes special characters in mcp name", async () => {
    const fetchMock = setupFetchMock({ success: true });

    const { deleteMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    await deleteMcp("mcp name");

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/mcp%20name");
    expect(calls[0][1].method).toBe("DELETE");
  });

  test("probeMcp calls POST /api/v1/admin/mcps/:name/probe", async () => {
    const probeResult = {
      status: "success",
      tools: [{ name: "read_file", description: "Read a file" }],
    };
    const fetchMock = setupFetchMock(probeResult);

    const { probeMcp } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    const result = await probeMcp("filesystem");

    expect(result.status).toBe("success");
    expect(result.tools).toHaveLength(1);
    expect(result.tools?.[0].name).toBe("read_file");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/filesystem/probe");
    expect(calls[0][1].method).toBe("POST");
  });

  test("probeMcpByConfig calls POST /api/v1/admin/mcps/probe with body", async () => {
    const probeResult = {
      status: "success",
      tools: [
        { name: "read_file", description: "Read a file" },
        { name: "write_file", description: "Write a file" },
      ],
    };
    const fetchMock = setupFetchMock(probeResult);

    const { probeMcpByConfig } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    const result = await probeMcpByConfig({
      transportType: "sse",
      url: "http://localhost:3001/sse",
    });

    expect(result.status).toBe("success");
    expect(result.tools).toHaveLength(2);
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/probe");
    expect(calls[0][1].method).toBe("POST");
    expect(calls[0][1].body.transportType).toBe("sse");
    expect(calls[0][1].body.url).toBe("http://localhost:3001/sse");
  });

  test("probeMcpByConfig returns failed status on error", async () => {
    const probeResult = {
      status: "failed",
      error: "Connection refused",
    };
    setupFetchMock(probeResult);

    const { probeMcpByConfig } = await import(
      `../../src/client/mcp.ts?t=${Date.now()}`
    );
    const result = await probeMcpByConfig({
      transportType: "sse",
      url: "http://invalid:9999/sse",
    });

    expect(result.status).toBe("failed");
    expect(result.error).toBe("Connection refused");
  });
});
