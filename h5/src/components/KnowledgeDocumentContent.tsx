import React, { useEffect, useState } from "react";
import {
  chunkPageNumbers,
  downloadOriginal,
  listDocumentChunks,
  loadChunkImage,
  type MobileChunk,
} from "../api/knowledge";

export function KnowledgeChunkImage({
  datasetId,
  imageId,
}: {
  datasetId: string;
  imageId: string;
}) {
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    let objectUrl = "";
    setUrl("");
    setError("");
    loadChunkImage(datasetId, imageId, controller.signal)
      .then((blob) => {
        if (controller.signal.aborted) return;
        objectUrl = URL.createObjectURL(blob);
        setUrl(objectUrl);
      })
      .catch((err) => {
        if (!controller.signal.aborted) setError(err.message);
      });
    return () => {
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [datasetId, imageId, retry]);
  return url ? (
    <img src={url} alt="切片图片" className="max-w-full rounded-lg" />
  ) : error ? (
    <div role="alert">
      {error}
      <button
        onClick={() => setRetry((value) => value + 1)}
        className="text-emerald-700 ml-2"
      >
        重试图片
      </button>
    </div>
  ) : (
    <p role="status">图片加载中…</p>
  );
}

export const KnowledgeChunkCard: React.FC<{
  chunk: MobileChunk;
  datasetId: string;
  onPage?: (page: number) => void;
}> = ({ chunk, datasetId, onPage }) => {
  const pages = chunkPageNumbers(chunk.positions);
  return (
    <article className="border border-gray-200 bg-white rounded-xl p-3 space-y-2">
      {!chunk.available && (
        <p className="text-amber-700">此切片已停用，不参与检索</p>
      )}
      <p className="whitespace-pre-wrap break-words leading-relaxed">
        {chunk.content || "切片没有文本内容"}
      </p>
      {chunk.imageId && (
        <KnowledgeChunkImage datasetId={datasetId} imageId={chunk.imageId} />
      )}
      {pages.length > 0 && (
        <div className="flex flex-wrap gap-1 text-gray-500">
          {pages.map((page) =>
            onPage ? (
              <button
                key={page}
                onClick={() => onPage(page)}
                className="text-emerald-700 px-2 rounded-lg border border-gray-200"
              >
                原文第 {page} 页
              </button>
            ) : (
              <span key={page}>第 {page} 页</span>
            ),
          )}
        </div>
      )}
    </article>
  );
};

export function KnowledgePager({
  page,
  hasNext,
  busy,
  onChange,
}: {
  page: number;
  hasNext: boolean;
  busy: boolean;
  onChange: (page: number) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-2 py-2">
      <button
        disabled={busy || page === 1}
        onClick={() => onChange(page - 1)}
        className="px-3 border border-gray-200 rounded-xl"
      >
        上一页
      </button>
      <span aria-live="polite">第 {page} 页</span>
      <button
        disabled={busy || !hasNext}
        onClick={() => onChange(page + 1)}
        className="px-3 border border-gray-200 rounded-xl"
      >
        下一页
      </button>
    </div>
  );
}

export const KnowledgeDocumentContent: React.FC<{
  datasetId: string;
  documentId: string;
  name: string;
  initialChunk?: MobileChunk;
}> = ({ datasetId, documentId, name, initialChunk }) => {
  const [tab, setTab] = useState<"chunks" | "original">("chunks");
  const [page, setPage] = useState(1);
  const [pdfPage, setPdfPage] = useState<number | null>(null);
  const [retry, setRetry] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [data, setData] = useState<{
    total: number;
    chunks: MobileChunk[];
  } | null>(null);
  const [original, setOriginal] = useState<{
    url: string;
    kind: "pdf" | "image" | "text" | "download";
    text?: string;
  } | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let objectUrl = "";
    setError("");
    setBusy(true);
    setData(null);
    setOriginal(null);
    const load = async () => {
      if (tab === "chunks") {
        const result = await listDocumentChunks(
          datasetId,
          documentId,
          page,
          controller.signal,
        );
        if (!controller.signal.aborted) setData(result);
      } else {
        const blob = await downloadOriginal(
          datasetId,
          documentId,
          controller.signal,
        );
        const signature = await blob.slice(0, 5).text();
        const isText =
          /\.(txt|md|markdown|csv|json|log)$/i.test(name) &&
          blob.size <= 2 * 1024 * 1024;
        const kind =
          signature === "%PDF-"
            ? "pdf"
            : /^image\/(png|jpeg|gif|webp|bmp)$/.test(blob.type)
              ? "image"
              : isText
                ? "text"
                : "download";
        const text = kind === "text" ? await blob.text() : undefined;
        if (controller.signal.aborted) return;
        objectUrl = URL.createObjectURL(
          kind === "pdf" ? new Blob([blob], { type: "application/pdf" }) : blob,
        );
        setOriginal({ url: objectUrl, kind, text });
      }
    };
    load()
      .catch((err) => {
        if (!controller.signal.aborted) setError(err.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => {
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [datasetId, documentId, name, tab, page, retry]);
  return (
    <section className="space-y-3">
      <div className="flex gap-2" aria-label="文档内容视图">
        <button
          aria-pressed={tab === "chunks"}
          onClick={() => setTab("chunks")}
          className={`flex-1 rounded-xl border border-gray-200 ${tab === "chunks" ? "bg-neutral-900 text-white" : ""}`}
        >
          切片核对
        </button>
        <button
          aria-pressed={tab === "original"}
          onClick={() => setTab("original")}
          className={`flex-1 rounded-xl border border-gray-200 ${tab === "original" ? "bg-neutral-900 text-white" : ""}`}
        >
          查看原文
        </button>
      </div>
      {tab === "chunks" && initialChunk && (
        <div className="space-y-2">
          <p className="font-semibold text-emerald-700">本次检索命中</p>
          <KnowledgeChunkCard
            chunk={initialChunk}
            datasetId={datasetId}
            onPage={
              /\.pdf$/i.test(name)
                ? (position) => {
                    setPdfPage(position);
                    setTab("original");
                  }
                : undefined
            }
          />
        </div>
      )}
      {busy && (
        <p role="status">
          {tab === "original" ? "原文加载中…" : "切片加载中…"}
        </p>
      )}
      {error && (
        <div role="alert" className="text-red-700">
          {error}
          <button
            onClick={() => setRetry((value) => value + 1)}
            className="ml-2 text-emerald-700"
          >
            重试读取
          </button>
        </div>
      )}
      {tab === "chunks" && !busy && data && (
        <>
          <p className="text-gray-500">共 {data.total} 个切片 · 每页 20 个</p>
          {data.chunks.length === 0 ? (
            <p>暂无切片。请核对解析是否完成，以及解析是否失败。</p>
          ) : (
            data.chunks.map((chunk) => (
              <KnowledgeChunkCard
                key={chunk.id}
                chunk={chunk}
                datasetId={datasetId}
                onPage={
                  /\.pdf$/i.test(name)
                    ? (position) => {
                        setPdfPage(position);
                        setTab("original");
                      }
                    : undefined
                }
              />
            ))
          )}
          <KnowledgePager
            page={page}
            hasNext={page * 20 < data.total}
            busy={busy}
            onChange={setPage}
          />
        </>
      )}
      {tab === "original" && original && (
        <>
          <a
            href={original.url}
            download={name}
            className="inline-flex items-center min-h-11 text-emerald-700 underline"
          >
            下载原文
          </a>
          {original.kind === "pdf" && (
            <>
              <p className="text-gray-500">
                {pdfPage
                  ? `切片来源第 ${pdfPage} 页，尝试打开对应页。`
                  : "尝试预览原文。"}
                手机浏览器可能忽略页锚或不支持预览，请下载核对。
              </p>
              <iframe
                title={`${name} 原文`}
                src={`${original.url}${pdfPage ? `#page=${pdfPage}` : ""}`}
                className="w-full h-80 border border-gray-200 rounded-lg"
              />
            </>
          )}
          {original.kind === "image" && (
            <img
              src={original.url}
              alt={name}
              className="max-w-full rounded-lg"
            />
          )}
          {original.kind === "text" && (
            <pre className="font-sans leading-relaxed text-sm">
              {original.text}
            </pre>
          )}
          {original.kind === "download" && (
            <p className="text-gray-500">
              此格式暂不支持手机内预览，请下载后用相应应用打开。
            </p>
          )}
        </>
      )}
    </section>
  );
};
