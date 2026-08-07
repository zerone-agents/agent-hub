import { describe, test, expect, mock, beforeEach } from "bun:test";

// Mocks the real /auth/userinfo response documented in
// internal/handler/auth.go::UserInfo:
//   { success: true, data: { id, username, email, display_name, tenant_id,
//                             org_id, avatar, roles, permissions } }
// `roles` is []*casdoorsdk.Role — i.e. { name, displayName, ... } objects.

import * as realConfig from "../../src/config";

describe("whoami command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({
          serverUrl: "https://test.local",
          token: "cli_abc123def456",
        })
      ),
      loadConfig: mock(() =>
        Promise.resolve({
          currentProfile: "default",
          profiles: {
            default: {
              serverUrl: "https://test.local",
              token: "cli_abc123def456",
            },
          },
        })
      ),
    }));
  });

  test("prints profile + unwrapped userinfo with role displayName", async () => {
    const fakeUserinfo = {
      success: true,
      data: {
        id: "user-42",
        username: "alice",
        email: "alice@example.com",
        display_name: "Alice Wang",
        tenant_id: "",
        org_id: "org-x",
        avatar: "",
        roles: [
          { name: "admin", displayName: "管理员" },
          { name: "ops", displayName: "运维" },
        ],
        permissions: [],
      },
    };

    mock.module("ofetch", () => ({
      ofetch: mock(() => Promise.resolve(fakeUserinfo)),
      FetchError: class FetchError extends Error {},
    }));

    const { WhoamiCommand } = await import(
      `../../src/commands/whoami.ts?t=${Date.now()}`
    );
    const cmd = new WhoamiCommand();

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("Profile:    default");
    expect(out).toContain("Server:     https://test.local");
    expect(out).toContain("cli_abc123de"); // first 12 chars of token
    expect(out).toContain("Alice Wang");
    expect(out).toContain("管理员,运维");
  });

  test("falls back to username when display_name missing; handles string roles", async () => {
    const fakeUserinfo = {
      success: true,
      data: {
        id: "user-7",
        username: "bob",
        roles: ["viewer"],
      },
    };

    mock.module("ofetch", () => ({
      ofetch: mock(() => Promise.resolve(fakeUserinfo)),
      FetchError: class FetchError extends Error {},
    }));

    const { WhoamiCommand } = await import(
      `../../src/commands/whoami.ts?t=${Date.now()}`
    );
    const cmd = new WhoamiCommand();

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      await cmd.execute();
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("bob (viewer)");
  });

  test("shows (无角色) when roles array is empty", async () => {
    const fakeUserinfo = {
      success: true,
      data: {
        id: "user-0",
        username: "nobody",
        roles: [],
      },
    };

    mock.module("ofetch", () => ({
      ofetch: mock(() => Promise.resolve(fakeUserinfo)),
      FetchError: class FetchError extends Error {},
    }));

    const { WhoamiCommand } = await import(
      `../../src/commands/whoami.ts?t=${Date.now()}`
    );
    const cmd = new WhoamiCommand();

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      await cmd.execute();
    } finally {
      console.log = origLog;
    }

    expect(logs.join("\n")).toContain("nobody ((无角色))");
  });
});
