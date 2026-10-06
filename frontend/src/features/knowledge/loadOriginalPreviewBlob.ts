import apiClient from "@/api/client";
import { chunkReviewText } from "./chunkReviewText";

export async function loadOriginalPreviewBlob(
  url: string,
  limit: number,
  signal: AbortSignal,
  textDocument = false,
): Promise<Blob> {
  // The fetch adapter resolves a stream when headers arrive. Keep the shared
  // Authorization/refresh interceptors while bounding reads before Blob creation.
  const response = await apiClient.get<ReadableStream<Uint8Array>>(url, {
    adapter: "fetch",
    responseType: "stream",
    headers: { Accept: "application/octet-stream, */*" },
    timeout: 120000,
    signal,
  });
  const stream = response.data;
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- empty/unsupported runtime adapters can return null despite the caller's generic
  if (!stream || typeof stream.getReader !== "function")
    throw new Error(chunkReviewText("badResponse"));
  const tooLarge = () =>
    new Error(chunkReviewText(textDocument ? "textTooLarge" : "tooLarge"));
  const declaredSize = Number(response.headers["content-length"]);
  if (Number.isFinite(declaredSize) && declaredSize > limit) {
    await stream.cancel().catch(() => undefined);
    throw tooLarge();
  }
  if (signal.aborted) {
    await stream.cancel().catch(() => undefined);
    throw new DOMException("Aborted", "AbortError");
  }
  const reader = stream.getReader();
  const parts: Uint8Array<ArrayBuffer>[] = [];
  let received = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- abort can occur while reader.read is pending
      if (signal.aborted) throw new DOMException("Aborted", "AbortError");
      if (done) break;
      received += value.byteLength;
      if (received > limit) throw tooLarge();
      // Own each bounded chunk; never retain a shared/transferable source buffer.
      parts.push(new Uint8Array(value));
    }
    const contentType = response.headers["content-type"];
    return new Blob(parts, {
      type:
        typeof contentType === "string"
          ? contentType
          : "application/octet-stream",
    });
  } finally {
    await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}
