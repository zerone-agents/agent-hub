import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Button,
  Empty,
  InputNumber,
  Space,
  Spin,
  Typography,
} from "antd";
import { DownloadSimpleIcon } from "@phosphor-icons/react";
import { createStyles } from "antd-style";
import { parseApiError } from "@/api/client";
import { loadOriginalPreviewBlob } from "./loadOriginalPreviewBlob";
import {
  knowledgeApi,
  type KnowledgeChunk,
  type KnowledgeDocument,
} from "@/api/knowledge";
import { tokens as t } from "@/styles/tokens";
import { chunkReviewText, useChunkReviewText } from "./chunkReviewText";

const PdfPreview = lazy(() => import("./KnowledgePdfPreview"));

const useStyles = createStyles(({ css }) => ({
  root: css`
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
  `,
  toolbar: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    flex-wrap: wrap;
  `,
  viewport: css`
    height: min(68vh, 760px);
    min-height: 280px;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: ${t.radiusSm}px;
    background: ${t.paper};
  `,
  text: css`
    padding: 18px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: ${t.text};
    font-family: inherit;
    font-size: ${t.textBase};
    line-height: 1.7;
    margin: 0;
    mark {
      background: var(--primary-soft);
      color: ${t.text};
      outline: 2px solid var(--primary);
      border-radius: 2px;
    }
  `,
  image: css`
    display: block;
    max-width: 100%;
    margin: auto;
  `,
  positions: css`
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    max-height: 100px;
    overflow: auto;
  `,
  state: css`
    min-height: 240px;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  `,
}));

export interface ChunkPosition {
  page: number;
  coordinates: number[];
}
export function chunkPositions(chunk?: KnowledgeChunk | null): ChunkPosition[] {
  return (chunk?.positions ?? []).flatMap((raw) => {
    // REST positions are [1-based page, left, right, top, bottom]. Scalar
    // legacy/unknown values are not page numbers and must not be guessed.
    if (
      !Array.isArray(raw) ||
      raw.length !== 5 ||
      !raw.every((v) => typeof v === "number" && Number.isFinite(v))
    )
      return [];
    const values = raw as number[];
    if (
      !Number.isInteger(values[0]) ||
      values[0] < 1 ||
      values[1] < 0 ||
      values[3] < 0 ||
      values[2] < values[1] ||
      values[4] < values[3]
    )
      return [];
    return [{ page: values[0], coordinates: values.slice(1) }];
  });
}

type PreviewKind = "pdf" | "text" | "image" | "unsupported";
const rasterTypes = new Set([
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
  "image/bmp",
]);
export function originalPreviewKind(
  doc: Pick<KnowledgeDocument, "name" | "suffix">,
): PreviewKind {
  const suffix =
    [doc.suffix, doc.name.split(".").pop()]
      .find(Boolean)
      ?.replace(/^\./, "")
      .toLowerCase() ?? "";
  if (suffix === "pdf") return "pdf";
  if (["png", "jpg", "jpeg", "gif", "webp", "bmp"].includes(suffix))
    return "image";
  if (
    [
      "txt",
      "md",
      "markdown",
      "csv",
      "tsv",
      "log",
      "json",
      "yaml",
      "yml",
      "xml",
      "html",
      "htm",
    ].includes(suffix)
  )
    return "text";
  return "unsupported";
}

interface Resource {
  url: string;
  text: string;
  blob: Blob;
  kind: PreviewKind;
}
export default function KnowledgeOriginalPreview({
  datasetId,
  documentId,
  document: doc,
  selectedChunk,
  locationRequest = 0,
}: {
  datasetId: string;
  documentId: string;
  document?: KnowledgeDocument | null;
  selectedChunk?: KnowledgeChunk | null;
  locationRequest?: number;
}) {
  // A keyed child owns every request and Blob URL; changed documents never
  // render the previous resource, including while the next effect starts.
  return (
    <OriginalResource
      key={`${datasetId}:${documentId}:${doc?.name ?? ""}`}
      datasetId={datasetId}
      documentId={documentId}
      document={doc}
      selectedChunk={selectedChunk}
      locationRequest={locationRequest}
    />
  );
}
function OriginalResource({
  datasetId,
  documentId,
  document: doc,
  selectedChunk,
  locationRequest = 0,
}: {
  datasetId: string;
  documentId: string;
  document?: KnowledgeDocument | null;
  selectedChunk?: KnowledgeChunk | null;
  locationRequest?: number;
}) {
  const text = useChunkReviewText();
  const { styles } = useStyles();
  const [resource, setResource] = useState<Resource | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [retry, setRetry] = useState(0);
  const [manualPage, setManualPage] = useState<{
    chunkId?: string;
    locationRequest?: number;
    page: number;
  }>({ page: 1 });
  const [pdfPageCount, setPdfPageCount] = useState(0);
  const [imageFailed, setImageFailed] = useState(false);
  const matchRef = useRef<HTMLElement>(null);
  const kind = doc ? originalPreviewKind(doc) : "unsupported";
  const positions = useMemo(
    () => chunkPositions(selectedChunk),
    [selectedChunk],
  );
  const requestedPage =
    manualPage.chunkId === selectedChunk?.id &&
    manualPage.locationRequest === locationRequest
      ? manualPage.page
      : (positions[0]?.page ?? 1);

  const page = pdfPageCount
    ? Math.min(requestedPage, pdfPageCount)
    : requestedPage;

  useEffect(() => {
    if (!doc?.id || kind === "unsupported") return;
    const controller = new AbortController();
    let active = true;
    let objectURL = "";
    // eslint-disable-next-line react-hooks/set-state-in-effect -- request lifecycle reset, paired with cleanup below
    setLoading(true);
    setResource(null);
    setError("");
    setImageFailed(false);
    setPdfPageCount(0);
    const previewLimit = kind === "text" ? 5 * 1024 * 1024 : 50 * 1024 * 1024;
    if (doc.size > previewLimit) {
      setError(chunkReviewText(kind === "text" ? "textTooLarge" : "tooLarge"));
      setLoading(false);
      return;
    }
    void loadOriginalPreviewBlob(
      knowledgeApi.documents.downloadUrl(datasetId, documentId),
      previewLimit,
      controller.signal,
      kind === "text",
    )
      .then(async (blob) => {
        if (!(blob instanceof Blob) || blob.size === 0)
          throw new Error(chunkReviewText("empty"));
        if (blob.size > 50 * 1024 * 1024)
          throw new Error(chunkReviewText("tooLarge"));
        const mime = blob.type.split(";")[0].toLowerCase();
        if (mime === "application/json" && kind !== "text")
          throw new Error(chunkReviewText("badResponse"));
        let sourceText = "";
        let detectedMime = mime;
        if (kind === "pdf") {
          const prefix = new Uint8Array(await blob.slice(0, 5).arrayBuffer());
          if (String.fromCharCode(...prefix) !== "%PDF-")
            throw new Error(chunkReviewText("badResponse"));
        } else if (kind === "image") {
          if (
            mime &&
            mime !== "application/octet-stream" &&
            !rasterTypes.has(mime)
          )
            throw new Error(chunkReviewText("badResponse"));
          const bytes = new Uint8Array(await blob.slice(0, 12).arrayBuffer());
          const ascii = String.fromCharCode(...bytes);
          detectedMime =
            bytes[0] === 137 && ascii.slice(1, 4) === "PNG"
              ? "image/png"
              : bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255
                ? "image/jpeg"
                : /^GIF8[79]a/.test(ascii)
                  ? "image/gif"
                  : ascii.startsWith("RIFF") && ascii.slice(8, 12) === "WEBP"
                    ? "image/webp"
                    : ascii.startsWith("BM")
                      ? "image/bmp"
                      : "";
          if (!detectedMime || (rasterTypes.has(mime) && detectedMime !== mime))
            throw new Error(chunkReviewText("badResponse"));
        } else {
          if (blob.size > 5 * 1024 * 1024)
            throw new Error(chunkReviewText("textTooLarge"));
          sourceText = await blob.text();
          // Never render a gateway business-error envelope as a source document.
          if (mime === "application/json") {
            try {
              const body: unknown = JSON.parse(sourceText);
              if (
                body &&
                typeof body === "object" &&
                "success" in body &&
                body.success === false
              )
                throw new Error(chunkReviewText("badResponse"));
            } catch (err) {
              if (
                err instanceof Error &&
                err.message === chunkReviewText("badResponse")
              )
                throw err;
            }
          }
        }
        if (!active) return;
        const safeMime = kind === "pdf" ? "application/pdf" : detectedMime;
        objectURL = URL.createObjectURL(
          safeMime !== mime ? new Blob([blob], { type: safeMime }) : blob,
        );
        setResource({ url: objectURL, text: sourceText, blob, kind });
      })
      .catch((err: unknown) => {
        if (active && !controller.signal.aborted) setError(parseApiError(err));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [datasetId, documentId, doc?.id, doc?.name, doc?.size, kind, retry]);

  const content = selectedChunk?.content.trim() ?? "";
  const matchAt =
    resource?.kind === "text" && content ? resource.text.indexOf(content) : -1;
  useEffect(() => {
    if (matchAt >= 0 && typeof matchRef.current?.scrollIntoView === "function")
      matchRef.current.scrollIntoView({ block: "center", behavior: "instant" });
  }, [selectedChunk?.id, matchAt, resource, locationRequest]);

  const download = async () => {
    try {
      const blob =
        resource?.blob ??
        (await knowledgeApi.documents.download(datasetId, documentId)).data;
      const url = URL.createObjectURL(blob);
      const anchor = window.document.createElement("a");
      anchor.href = url;
      anchor.download = doc?.name ?? `document-${documentId}`;
      anchor.click();
      window.setTimeout(() => {
        URL.revokeObjectURL(url);
      }, 0);
    } catch (err) {
      setError(parseApiError(err));
    }
  };
  return (
    <section aria-label={text("original")} className={styles.root}>
      <div className={styles.toolbar}>
        <Typography.Text strong>{text("original")}</Typography.Text>
        <Button
          size="small"
          disabled={!doc}
          icon={<DownloadSimpleIcon size={16} />}
          onClick={() => void download()}
        >
          {text("download")}
        </Button>
      </div>
      {doc ? (
        <Typography.Text type="secondary" ellipsis={{ tooltip: doc.name }}>
          {doc.name}
        </Typography.Text>
      ) : null}
      {!doc ? (
        <Empty description={text("noDocument")} />
      ) : kind === "unsupported" ? (
        <Empty description={text("unsupported")} />
      ) : loading ? (
        <div className={styles.state} role="status">
          <Space>
            <Spin size="small" />
            {text("loading")}
          </Space>
        </div>
      ) : error ? (
        <Alert
          type="error"
          title={text("failed")}
          description={error}
          action={
            <Button
              onClick={() => {
                setRetry((v) => v + 1);
              }}
            >
              {text("retry")}
            </Button>
          }
        />
      ) : resource ? (
        <>
          {resource.kind === "pdf" ? (
            <>
              <Space wrap>
                <label htmlFor={`original-page-${documentId}`}>
                  {text("page")}
                </label>
                <InputNumber
                  id={`original-page-${documentId}`}
                  aria-label={text("page")}
                  min={1}
                  max={pdfPageCount || undefined}
                  precision={0}
                  value={page}
                  onChange={(next) => {
                    if (next)
                      setManualPage({
                        chunkId: selectedChunk?.id,
                        locationRequest,
                        page: next,
                      });
                  }}
                />
              </Space>
              {positions.length ? (
                <div className={styles.positions} aria-label={text("position")}>
                  {positions.map((position, index) => (
                    <Button
                      key={index}
                      size="small"
                      title={position.coordinates.join(", ")}
                      onClick={() => {
                        setManualPage({
                          chunkId: selectedChunk?.id,
                          locationRequest,
                          page: position.page,
                        });
                      }}
                    >
                      {text("page")} {position.page} ·{" "}
                      {position.coordinates.join(", ")}
                    </Button>
                  ))}
                </div>
              ) : selectedChunk ? (
                <Typography.Text type="secondary">
                  {text("noPosition")}
                </Typography.Text>
              ) : null}
              {pdfPageCount > 0 && requestedPage > pdfPageCount ? (
                <Typography.Text type="warning">
                  {text("pageOutside")}
                </Typography.Text>
              ) : null}
              <Typography.Text type="secondary">
                {text("pdfNote")}
              </Typography.Text>
            </>
          ) : null}
          {resource.kind === "text" && selectedChunk ? (
            <Typography.Text role="status" type="secondary">
              {text(matchAt >= 0 ? "found" : "notFound")}
            </Typography.Text>
          ) : null}
          <div
            className={resource.kind === "pdf" ? undefined : styles.viewport}
          >
            {resource.kind === "pdf" ? (
              <Suspense fallback={<Spin size="small" />}>
                <PdfPreview
                  blob={resource.blob}
                  page={page}
                  positions={positions}
                  onPageCount={setPdfPageCount}
                />
              </Suspense>
            ) : resource.kind === "image" ? (
              imageFailed ? (
                <Alert
                  type="error"
                  title={text("imageFailed")}
                  action={
                    <Button
                      onClick={() => {
                        setRetry((v) => v + 1);
                      }}
                    >
                      {text("retry")}
                    </Button>
                  }
                />
              ) : (
                <img
                  className={styles.image}
                  src={resource.url}
                  alt={doc.name}
                  onError={() => {
                    setImageFailed(true);
                  }}
                />
              )
            ) : (
              <pre className={styles.text}>
                {matchAt < 0 ? (
                  resource.text
                ) : (
                  <>
                    {resource.text.slice(0, matchAt)}
                    <mark ref={matchRef}>
                      {resource.text.slice(matchAt, matchAt + content.length)}
                    </mark>
                    {resource.text.slice(matchAt + content.length)}
                  </>
                )}
              </pre>
            )}
          </div>
        </>
      ) : null}
      {!selectedChunk ? (
        <Typography.Text type="secondary">{text("choose")}</Typography.Text>
      ) : null}
    </section>
  );
}
