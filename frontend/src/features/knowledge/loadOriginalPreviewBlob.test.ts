import { beforeEach, describe, expect, it, vi } from "vitest";
import { loadOriginalPreviewBlob } from "./loadOriginalPreviewBlob";
const h = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("@/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/client")>();
  return { ...actual, default: { ...actual.default, get: h.get } };
});
beforeEach(() => h.get.mockReset());
describe("bounded original preview transport", () => {
  it("cancels a known oversized response before acquiring or reading its body", async () => {
    const cancel = vi.fn().mockResolvedValue(undefined);
    const getReader = vi.fn();
    h.get.mockResolvedValue({
      data: { cancel, getReader },
      headers: { "content-length": "1000" },
    });
    await expect(
      loadOriginalPreviewBlob("/download", 10, new AbortController().signal),
    ).rejects.toThrow("50 MB");
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(getReader).not.toHaveBeenCalled();
    expect(h.get).toHaveBeenCalledWith(
      "/download",
      expect.objectContaining({
        adapter: "fetch",
        responseType: "stream",
        signal: expect.any(AbortSignal),
      }),
    );
  });
  it.each([{}, { "content-length": "1" }])(
    "cancels unknown or understated lengths once received bytes exceed the cap (%j)",
    async (headers) => {
      const read = vi
        .fn()
        .mockResolvedValueOnce({ done: false, value: new Uint8Array(8) })
        .mockResolvedValueOnce({ done: false, value: new Uint8Array(8) });
      const cancel = vi.fn().mockResolvedValue(undefined);
      const releaseLock = vi.fn();
      h.get.mockResolvedValue({
        data: { getReader: () => ({ read, cancel, releaseLock }) },
        headers,
      });
      await expect(
        loadOriginalPreviewBlob(
          "/download",
          10,
          new AbortController().signal,
          true,
        ),
      ).rejects.toThrow("5 MB");
      expect(read).toHaveBeenCalledTimes(2);
      expect(cancel).toHaveBeenCalledTimes(1);
      expect(releaseLock).toHaveBeenCalledTimes(1);
    },
  );
  it("materializes only a completed bounded body and preserves its MIME", async () => {
    const read = vi
      .fn()
      .mockResolvedValueOnce({ done: false, value: new Uint8Array([65, 66]) })
      .mockResolvedValueOnce({ done: true });
    const cancel = vi.fn().mockResolvedValue(undefined);
    h.get.mockResolvedValue({
      data: { getReader: () => ({ read, cancel, releaseLock: vi.fn() }) },
      headers: { "content-type": "text/plain", "content-length": "2" },
    });
    const blob = await loadOriginalPreviewBlob(
      "/download",
      10,
      new AbortController().signal,
      true,
    );
    expect(blob.size).toBe(2);
    expect(blob.type).toBe("text/plain");
    expect(cancel).toHaveBeenCalledTimes(1);
  });
});
