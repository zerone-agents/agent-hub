import { useEffect, useMemo, useRef, useState } from "react";
import { Alert, Button, Space, Spin, Typography } from "antd";
import { createStyles } from "antd-style";
import {
  getDocument,
  GlobalWorkerOptions,
  type PDFDocumentProxy,
  type PDFDocumentLoadingTask,
  type RenderTask,
} from "pdfjs-dist/legacy/build/pdf.mjs";
import workerUrl from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";
import { parseApiError } from "@/api/client";
import type { ChunkPosition } from "./KnowledgeOriginalPreview";
import { pdfHighlightRect } from "./pdfChunkPosition";
import { useChunkReviewText } from "./chunkReviewText";

GlobalWorkerOptions.workerSrc = workerUrl;
const useStyles = createStyles(({ css }) => ({
  root: css`
    min-width: 0;
  `,
  viewport: css`
    max-height: min(65vh, 760px);
    min-height: 240px;
    overflow: auto;
    padding: 12px;
    background: var(--muted);
    border-radius: 8px;
  `,
  page: css`
    position: relative;
    margin: 0 auto;
    box-shadow: var(--elevation-1);
    background: white;
  `,
  highlight: css`
    position: absolute;
    pointer-events: none;
    border: 2px solid #b54708;
    background: rgba(255, 180, 90, 0.22);
    box-sizing: border-box;
  `,
}));
export default function KnowledgePdfPreview({
  blob,
  page,
  positions,
  onPageCount,
}: {
  blob: Blob;
  page: number;
  positions: ChunkPosition[];
  onPageCount: (count: number) => void;
}) {
  const text = useChunkReviewText();
  const { styles } = useStyles();
  const canvas = useRef<HTMLCanvasElement>(null);
  const container = useRef<HTMLDivElement>(null);
  const [pdf, setPdf] = useState<PDFDocumentProxy | null>(null);
  const [error, setError] = useState("");
  const [rendering, setRendering] = useState(true);
  const [width, setWidth] = useState(600);
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);
  const [pageText, setPageText] = useState("");
  const [retry, setRetry] = useState(0);
  const [layout, setLayout] = useState({
    width: 0,
    height: 0,
    baseWidth: 0,
    baseHeight: 0,
    scale: 1,
  });
  useEffect(() => {
    const element = container.current;
    if (!element) return;
    const observer = new ResizeObserver((entries) => {
      setWidth(Math.max(100, entries[0].contentRect.width - 24));
    });
    observer.observe(element);
    return () => {
      observer.disconnect();
    };
  }, []);
  useEffect(() => {
    let active = true;
    let task: PDFDocumentLoadingTask | undefined;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset PDF request ownership, cancellation handled below
    setPdf(null);
    setError("");
    setRendering(true);
    void blob
      .arrayBuffer()
      .then((data) => {
        if (!active) return;
        // Parse only the already-authorized binary. Workers never receive a token.
        task = getDocument({
          data: new Uint8Array(data),
        });
        return task.promise;
      })
      .then((document) => {
        if (!active || !document) return;
        setPdf(document);
        onPageCount(document.numPages);
      })
      .catch((err: unknown) => {
        if (active) {
          setError(parseApiError(err));
          setRendering(false);
        }
      });
    return () => {
      active = false;
      if (task) void task.destroy().catch(() => undefined);
    };
  }, [blob, retry, onPageCount]);
  useEffect(() => {
    if (!pdf) return;
    let active = true;
    let task: RenderTask | undefined;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset the canvas render lifecycle on page/zoom/rotation changes
    setRendering(true);
    setError("");
    void pdf
      .getPage(Math.min(Math.max(1, page), pdf.numPages))
      .then(async (pdfPage) => {
        if (!active || !canvas.current) return;
        const base = pdfPage.getViewport({ scale: 1 });
        const rotated = pdfPage.getViewport({
          scale: 1,
          rotation: (pdfPage.rotate + rotation) % 360,
        });
        const scale = (width / rotated.width) * zoom;
        const viewport = pdfPage.getViewport({
          scale,
          rotation: (pdfPage.rotate + rotation) % 360,
        });
        // Bound HiDPI memory while preserving CSS page dimensions.
        const outputScale = Math.min(
          window.devicePixelRatio || 1,
          2,
          Math.sqrt(16000000 / (viewport.width * viewport.height)),
        );
        const element = canvas.current;
        element.width = Math.ceil(viewport.width * outputScale);
        element.height = Math.ceil(viewport.height * outputScale);
        element.style.width = `${viewport.width}px`;
        element.style.height = `${viewport.height}px`;
        const context = element.getContext("2d");
        if (!context) throw new Error("Canvas unavailable");
        setLayout({
          width: viewport.width,
          height: viewport.height,
          baseWidth: base.width,
          baseHeight: base.height,
          scale,
        });
        task = pdfPage.render({
          canvas: element,
          canvasContext: context,
          viewport,
          transform: [outputScale, 0, 0, outputScale, 0, 0],
        });
        await task.promise;
        const content = await pdfPage.getTextContent();
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- request cleanup can run while awaiting render/text promises
        if (!active) return;
        setPageText(
          content.items
            .flatMap((item) => ("str" in item ? [item.str] : []))
            .join(" "),
        );
        setRendering(false);
      })
      .catch((err: unknown) => {
        if (active) {
          setError(parseApiError(err));
          setRendering(false);
        }
      });
    return () => {
      active = false;
      task?.cancel();
    };
  }, [pdf, page, width, zoom, rotation, retry]);
  const pagePositions = useMemo(
    () => positions.filter((position) => position.page === page),
    [positions, page],
  );
  const rectangles = pagePositions
    .map((position) =>
      pdfHighlightRect(
        position.coordinates,
        layout.baseWidth,
        layout.baseHeight,
        rotation,
        layout.scale,
      ),
    )
    .filter((rect) => rect !== null);
  return (
    <div className={styles.root}>
      <Space wrap style={{ marginBottom: 8 }}>
        <Button
          size="small"
          aria-label={text("zoomOut")}
          disabled={zoom <= 0.5}
          onClick={() => {
            setZoom((value) => Math.max(0.5, value - 0.25));
          }}
        >
          −
        </Button>
        <Typography.Text>{Math.round(zoom * 100)}%</Typography.Text>
        <Button
          size="small"
          aria-label={text("zoomIn")}
          disabled={zoom >= 2}
          onClick={() => {
            setZoom((value) => Math.min(2, value + 0.25));
          }}
        >
          +
        </Button>
        <Button
          size="small"
          onClick={() => {
            setRotation((value) => (value + 90) % 360);
          }}
        >
          {text("rotate")}
        </Button>
        {pdf ? (
          <Typography.Text>
            {page} / {pdf.numPages}
          </Typography.Text>
        ) : null}
      </Space>
      {error ? (
        <Alert
          type="error"
          title={text("pdfFailed")}
          description={error}
          action={
            <Button
              aria-label={text("retry")}
              onClick={() => {
                setRetry((value) => value + 1);
              }}
            >
              {text("retry")}
            </Button>
          }
        />
      ) : null}
      {rendering ? (
        <Space role="status" style={{ marginBottom: 8 }}>
          <Spin size="small" />
          {text("pdfLoading")}
        </Space>
      ) : null}
      {!rendering && !error && pagePositions.length > rectangles.length ? (
        <Typography.Paragraph type="secondary">
          {text("positionOutside")}
        </Typography.Paragraph>
      ) : null}
      <div className={styles.viewport} ref={container}>
        <div
          className={styles.page}
          style={{
            width: layout.width || "100%",
            height: layout.height || undefined,
            visibility: rendering || error ? "hidden" : "visible",
          }}
        >
          {!rendering && !error ? (
            <p
              style={{
                position: "absolute",
                width: 1,
                height: 1,
                overflow: "hidden",
                clipPath: "inset(50%)",
              }}
            >
              {pageText}
            </p>
          ) : null}
          <canvas
            ref={canvas}
            role="img"
            aria-label={`${text("original")} · ${text("page")} ${page}`}
          />
          {rectangles.map((rect, index) => (
            <div
              key={index}
              aria-hidden
              className={styles.highlight}
              data-pdf-highlight
              style={rect}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
