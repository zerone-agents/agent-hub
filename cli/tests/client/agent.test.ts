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

describe("agent client", () => {
  beforeEach(setupConfigMock);
  afterEach(() => mock.restore());

  test("setAgentMcps sends PUT /api/v1/admin/agents/:name/mcps", async () => {
    const fetchMock = setupFetchMock({ success: true });
    const { setAgentMcps } = await import(
      `../../src/client/agent.ts?t=${Date.now()}`
    );
    await setAgentMcps("research-agent", ["knowledge", "web-search"]);
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/agents/research-agent/mcps");
    expect(calls[0][1].method).toBe("PUT");
    expect(calls[0][1].body).toEqual({
      mcpNames: ["knowledge", "web-search"],
    });
  });
});
