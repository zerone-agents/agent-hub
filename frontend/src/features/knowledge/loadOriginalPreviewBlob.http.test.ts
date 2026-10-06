import { createServer } from "node:http";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { loadOriginalPreviewBlob } from "./loadOriginalPreviewBlob";

const observations = new Map<
  string,
  { bytes: number; authorization?: string; closed: Promise<void> }
>();
const server = createServer((request, response) => {
  const path = request.url ?? "";
  let closed!: () => void;
  const record = {
    bytes: 0,
    authorization: request.headers.authorization,
    closed: new Promise<void>((resolve) => {
      closed = resolve;
    }),
  };
  observations.set(path, record);
  if (path === "/small") {
    response.writeHead(200, {
      "Content-Type": "text/plain",
      "Content-Length": "2",
    });
    record.bytes = 2;
    response.end("OK");
    response.once("close", closed);
    return;
  }
  response.writeHead(200, {
    "Content-Type": "application/pdf",
    ...(path === "/large" ? { "Content-Length": "65536" } : {}),
  });
  response.flushHeaders();
  const timer = setInterval(() => {
    record.bytes += 512;
    response.write(Buffer.alloc(512, 65));
  }, 10);
  response.once("close", () => {
    clearInterval(timer);
    closed();
  });
});
let base = "";
beforeAll(async () => {
  await new Promise<void>((resolve) => {
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string")
    throw new Error("Missing HTTP fixture address");
  base = `http://127.0.0.1:${address.port}`;
});
afterEach(() => {
  localStorage.removeItem("access_token");
});
afterAll(async () => {
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) => {
    server.close((error) => {
      if (error) reject(error);
      else resolve();
    });
  });
});
describe("original preview through real authenticated HTTP fetch transport", () => {
  it("checks declared size at headers and closes the oversized response without downloading the full file", async () => {
    localStorage.setItem("access_token", "preview-test-token");
    await expect(
      loadOriginalPreviewBlob(
        `${base}/large`,
        1024,
        new AbortController().signal,
      ),
    ).rejects.toThrow("50 MB");
    const record = observations.get("/large")!;
    await record.closed;
    expect(record.authorization).toBe("Bearer preview-test-token");
    expect(record.bytes).toBeLessThan(65536);
  });
  it("bounds and closes an unknown-length chunked response while reading", async () => {
    await expect(
      loadOriginalPreviewBlob(
        `${base}/unknown`,
        1024,
        new AbortController().signal,
      ),
    ).rejects.toThrow("50 MB");
    const record = observations.get("/unknown")!;
    await record.closed;
    expect(record.bytes).toBeGreaterThan(1024);
    expect(record.bytes).toBeLessThan(65536);
  });
  it("preserves a complete small original with the real fetch adapter", async () => {
    const blob = await loadOriginalPreviewBlob(
      `${base}/small`,
      1024,
      new AbortController().signal,
      true,
    );
    expect(blob.size).toBe(2);
    expect(blob.type).toBe("text/plain");
  });
});
