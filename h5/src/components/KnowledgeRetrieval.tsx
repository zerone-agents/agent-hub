import React, { useEffect, useState } from "react";
import { KnowledgeDocument } from "../types";
import { searchKnowledge, type MobileChunk } from "../api/knowledge";
import { KnowledgeChunkCard, KnowledgePager } from "./KnowledgeDocumentContent";

export const KnowledgeRetrieval: React.FC<{
  datasetId: string;
  documents: KnowledgeDocument[];
  onOpenDocument: (doc: KnowledgeDocument, chunk?: MobileChunk) => void;
}> = ({ datasetId, documents, onOpenDocument }) => {
  const [question, setQuestion] = useState("");
  const [documentId, setDocumentId] = useState("");
  const [submitted, setSubmitted] = useState<{
    question: string;
    documentId: string;
    page: number;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<{
    total: number;
    chunks: MobileChunk[];
  } | null>(null);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!submitted) return;
    const controller = new AbortController();
    setBusy(true);
    setError("");
    setResult(null);
    searchKnowledge(
      datasetId,
      submitted.question,
      submitted.page,
      submitted.documentId || undefined,
      controller.signal,
    )
      .then((data) => {
        if (!controller.signal.aborted) setResult(data);
      })
      .catch((err) => {
        if (!controller.signal.aborted) setError(err.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [datasetId, submitted, retry]);
  return (
    <section className="space-y-3 text-sm">
      <p className="text-gray-500">
        输入资料里应能回答的问题，核对命中的内容与来源。解析完成且有切片后再验证。
      </p>
      <form
        className="space-y-3"
        onSubmit={(event) => {
          event.preventDefault();
          if (question.trim())
            setSubmitted({ question: question.trim(), documentId, page: 1 });
        }}
      >
        <label className="block">
          要核对的问题
          <textarea
            required
            rows={2}
            value={question}
            onChange={(event) => setQuestion(event.target.value)}
            onKeyDown={(event) => {
              if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
            placeholder="例如：报销需要哪些材料？"
            className="mt-1 w-full border border-gray-200 rounded-xl p-3 bg-white"
          />
        </label>
        <label className="block">
          检索范围
          <select
            value={documentId}
            onChange={(event) => setDocumentId(event.target.value)}
            className="mt-1 w-full border border-gray-200 rounded-xl p-2 bg-white"
          >
            <option value="">当前知识库全部资料</option>
            {documents.map((doc) => (
              <option value={doc.id} key={doc.id}>
                {doc.name}
              </option>
            ))}
          </select>
        </label>
        <button
          type="submit"
          disabled={busy || !question.trim()}
          className="w-full rounded-xl bg-neutral-900 text-white"
        >
          {busy ? "检索中…" : "验证检索"}
        </button>
      </form>
      {error && (
        <div role="alert" className="text-red-700">
          {error}
          <button
            onClick={() => setRetry((value) => value + 1)}
            className="ml-2 text-emerald-700"
          >
            重试检索
          </button>
        </div>
      )}
      {result && submitted && (
        <div className="space-y-3">
          <p role="status" className="text-gray-500">
            上次验证：“{submitted.question}” ·{" "}
            {submitted.documentId
              ? documents.find((doc) => doc.id === submitted.documentId)
                  ?.name || "指定文档"
              : "当前知识库全部资料"}{" "}
            · 命中 {result.total} 个切片
          </p>
          {!result.chunks.length && (
            <div className="border border-gray-200 rounded-xl p-3">
              <p>没有找到相关内容。</p>
              <p className="text-gray-500 mt-2">
                核对解析状态和切片数，换一个资料中出现的具体问题，或调整检索范围后重试。
              </p>
            </div>
          )}
          {result.chunks.map((chunk) => {
            const source = documents.find((doc) => doc.id === chunk.documentId);
            return (
              <div key={chunk.id} className="space-y-1">
                <div className="flex items-center justify-between gap-2">
                  <button
                    disabled={!source}
                    onClick={() => {
                      if (source) onOpenDocument(source, chunk);
                    }}
                    className="text-emerald-700 text-left break-all"
                  >
                    {chunk.documentName || source?.name || "来源文档不可用"}
                    {source ? " · 核对原文" : ""}
                  </button>
                  {chunk.similarity != null && (
                    <span className="text-gray-500 shrink-0 text-xs">
                      相似度 {chunk.similarity.toFixed(2)}
                    </span>
                  )}
                </div>
                <KnowledgeChunkCard chunk={chunk} datasetId={datasetId} />
              </div>
            );
          })}
          <KnowledgePager
            page={submitted.page}
            busy={busy}
            hasNext={submitted.page * 10 < result.total}
            onChange={(page) => setSubmitted({ ...submitted, page })}
          />
        </div>
      )}
    </section>
  );
};
