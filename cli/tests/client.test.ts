import { describe, test, expect, mock, beforeEach, afterEach } from "bun:test";

// Track the profile that the mocked config module will return. Tests can
// mutate this before each call.
const profileSlot: { serverUrl: string; token: string } = {
  serverUrl: "https://test.local",
  token: "cli_test",
};

// Use mock.module so the real src/client/base.ts (which imports from
// "../config") receives a stubbed getActiveProfile. The plan's approach of
// assigning to `configMod.getActiveProfile` fails under Bun because the
// ES module namespace is frozen (read-only exports).
//
// IMPORTANT: Bun's mock.module replaces the ENTIRE module namespace for the
// whole test process, so tests that import other exports from ../src/config.ts
// (e.g. login.test.ts using loadConfig/saveConfig) would break if we only
// returned getActiveProfile here. Re-export the real surface and override
// just the function under test.
import * as realConfig from "../src/config.ts";
mock.module("../src/config.ts", () => ({
  ...realConfig,
  getActiveProfile: () => Promise.resolve({ ...profileSlot }),
}));

// Helper: stub ofetch for the next dynamic import of base.ts. Because
// mock.module is process-scoped in Bun, the stub leaks between tests; each
// test therefore re-stubs ofetch with its own behavior. We export the
// FetchError class alongside ofetch so apiRequest's `e instanceof FetchError`
// check still works when our stubs throw.
function stubOfetch(
  impl: (url: string, init?: any) => unknown,
  FetchErrorImpl: new (msg?: string) => any = class FetchError extends Error {}
) {
  mock.module("ofetch", () => ({
    ofetch: mock((url: string, init?: any) => impl(url, init)),
    FetchError: FetchErrorImpl,
  }));
}

describe("apiRequest", () => {
  let origExit: (code?: number) => never;

  beforeEach(() => {
    origExit = process.exit;
    profileSlot.serverUrl = "https://test.local";
    profileSlot.token = "cli_test";
  });

  afterEach(() => {
    process.exit = origExit;
  });

  test("adds Authorization header from profile", async () => {
    let capturedInit: any;
    stubOfetch((url: string, init?: any) => {
      capturedInit = init;
      return { data: "ok" };
    });

    const { apiRequest } = await import(`../src/client/base.ts?t=${Date.now()}`);
    await apiRequest("/api/v1/agents");

    expect(capturedInit.headers.Authorization).toBe("Bearer cli_test");
    expect(capturedInit.method).toBe("GET");
  });

  test("auto-unwraps {success, data} envelope", async () => {
    stubOfetch(() =>
      Promise.resolve({ success: true, data: { agents: [{ id: 1 }] } })
    );

    const { apiRequest } = await import(`../src/client/base.ts?t=${Date.now()}`);
    const result: { agents: { id: number }[] } = await apiRequest(
      "/api/v1/agents"
    );
    expect(result).toEqual({ agents: [{ id: 1 }] });
  });

  test("returns raw payload when response is not enveloped", async () => {
    stubOfetch(() => Promise.resolve({ hello: "world" }));

    const { apiRequest } = await import(`../src/client/base.ts?t=${Date.now()}`);
    const result: { hello: string } = await apiRequest("/x");
    expect(result).toEqual({ hello: "world" });
  });

  test("exits with code 4 on 403", async () => {
    process.exit = ((code?: number) => {
      throw new Error(`exit:${code}`);
    }) as typeof process.exit;

    // Local FetchError subclass the stubbed ofetch module will export, so
    // apiRequest's `e instanceof FetchError` branch fires and it can read
    // `e.response.status` / `e.data.error`.
    class FetchError403 extends Error {
      response = { status: 403, data: { error: "forbidden" } };
      data = { error: "forbidden" };
    }

    stubOfetch(() => {
      throw new FetchError403();
    }, FetchError403 as any);

    const { apiRequest } = await import(`../src/client/base.ts?t=${Date.now()}`);
    await expect(apiRequest("/x")).rejects.toThrow("exit:4");
  });
});
