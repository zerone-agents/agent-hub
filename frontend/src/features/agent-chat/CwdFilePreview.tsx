import { useEffect, useRef, useState } from 'react'
import { message } from 'antd'
import { createStyles } from 'antd-style'
import { DownloadIcon, WarningIcon } from '@phosphor-icons/react'
import { agentFilesApi, type FileEntry } from '@/api/agent-files'
import { parseApiError } from '@/api/client'
import { tokens as t } from '@/styles/tokens'

/**
 * CwdFilePreview — inline preview + download for a single selected file.
 *
 * The preview is capped at PREVIEW_BYTE_CAP. Files larger than that show a
 * "文件较大" notice with a download button instead of attempting to render.
 *
 * Security notes:
 *  - Text content (including text/html) is rendered inside <pre> as escaped
 *    text. We never use dangerouslySetInnerHTML, so HTML files cannot render
 *    as live DOM.
 *  - Image / PDF previews are loaded via blob: URLs so the bearer token is
 *    not leaked in the URL; URLs are revoked on switch and unmount.
 */

const PREVIEW_BYTE_CAP = 512 * 1024 // 512 KB

const useStyles = createStyles(({ css }) => ({
  root: css`
    flex: 1 1 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: ${t.surface};
  `,
  placeholder: css`
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: ${t.textMuted};
    font-size: 12px;
  `,
  header: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 6px 10px;
    border-bottom: 1px solid ${t.inkLighter};
    font-size: 12px;
    color: ${t.textSecondary};
    background: ${t.surface};
  `,
  filename: css`
    font-family: ${t.fontMono};
    color: ${t.text};
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    flex: 1 1 auto;
  `,
  downloadLink: css`
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    border: none;
    border-radius: ${t.radiusSm};
    background: transparent;
    color: ${t.ink};
    text-decoration: none;
    font-family: inherit;
    font-size: 12px;
    cursor: pointer;
    &:hover {
      background: ${t.inkLight};
    }
  `,
  body: css`
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    padding: 8px 10px;
    font-family: ${t.fontMono};
    font-size: 12.5px;
    color: ${t.text};
  `,
  pre: css`
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: ${t.fontMono};
    font-size: 12.5px;
  `,
  notice: css`
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 32px 16px;
    color: ${t.textSecondary};
    text-align: center;
  `,
  noticeIcon: css`
    color: ${t.warning};
  `,
  noticeText: css`
    font-size: 13px;
  `,
  truncated: css`
    padding: 4px 10px;
    background: ${t.inkSubtle};
    color: ${t.textTertiary};
    font-size: 11px;
    border-bottom: 1px solid ${t.inkLighter};
  `,
  embedded: css`
    width: 100%;
    height: 100%;
    border: 0;
    display: block;
  `,
  pdfObject: css`
    width: 100%;
    height: 100%;
    border: 0;
    display: block;
  `,
}))

interface Props {
  agentName: string
  selectedFile: FileEntry | null
}

interface PreviewState {
  loading: boolean
  error: string | null
  text: string | null
  truncated: boolean
  objectUrl: string | null
  kind: 'text' | 'image' | 'pdf' | 'binary' | 'too-large' | null
  mime: string | null
  downloadName: string | null
}

const initialState: PreviewState = {
  loading: false,
  error: null,
  text: null,
  truncated: false,
  objectUrl: null,
  kind: null,
  mime: null,
  downloadName: null,
}

/**
 * The Task 8 panel rewrites the entry so that `name` already contains the
 * cwd-relative path (e.g. "src/index.ts"). The full request path is therefore
 * just the name. We isolate this in a helper so the panel can swap it later.
 */
function fullPath(entry: FileEntry): string {
  return entry.name
}

/**
 * Parse RFC 5987 `filename*=UTF-8''<percent-encoded>` first, fall back to the
 * bare `filename="..."` form. Returns null when nothing parseable is found,
 * in which case callers should use entry.name.
 */
export function parseFilenameFromContentDisposition(cd: string | null): string | null {
  if (!cd) return null
  // RFC 5987 form: filename*=UTF-8''<pct-encoded>
  const star = /filename\*=([^']*)'([^']*)'([^;]+)/i.exec(cd)
  if (star) {
    try {
      return decodeURIComponent(star[3])
    } catch {
      return star[3]
    }
  }
  // Plain quoted form: filename="..."
  const quoted = /filename="?([^";]+)"?/i.exec(cd)
  if (quoted) return quoted[1]
  return null
}

/**
 * Prettify JSON for display. Only attempts parse when the file's mime is
 * application/json OR the extension is .json. On parse failure (or non-JSON)
 * the original text is returned untouched. No third-party highlighter.
 */
export function prettifyIfJson(entry: FileEntry, text: string): string {
  const isJsonMime = entry.mime === 'application/json'
  const isJsonExt = /\.json$/i.test(entry.name)
  if (!isJsonMime && !isJsonExt) return text
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

/**
 * Determine preview kind from mime. Unknown / non-text mime falls back to
 * 'binary' so we render a "不支持预览" notice instead of attempting to
 * decode arbitrary bytes as UTF-8 text (which would produce mojibake).
 *
 * The recognized text mime families are:
 *  - text/*                         (covers text/plain, text/html, text/markdown, …)
 *  - application/json               (with optional +suffix, e.g. application/json+schema)
 *  - application/javascript and application/typescript
 *  - application/xml and application/xhtml+xml
 *  - image/svg+xml                  (renders as text)
 */
const TEXT_MIME_PREFIXES = ['text/']
const TEXT_MIME_EXACT = new Set([
  'application/json',
  'application/javascript',
  'application/typescript',
  'application/xml',
  'application/xhtml+xml',
  'image/svg+xml',
])

function kindFromMime(mime: string | undefined): 'text' | 'image' | 'pdf' | 'binary' {
  if (!mime) return 'text' // unknown — caller treats as text per legacy contract
  const lower = mime.toLowerCase()
  if (lower === 'application/pdf') return 'pdf'
  if (lower.startsWith('image/') && lower !== 'image/svg+xml') return 'image'
  if (TEXT_MIME_PREFIXES.some((p) => lower.startsWith(p))) return 'text'
  // application/json+foo etc.
  const base = lower.split('+')[0]
  if (TEXT_MIME_EXACT.has(base) || TEXT_MIME_EXACT.has(lower)) return 'text'
  return 'binary'
}

/**
 * Stream-read a text Response up to PREVIEW_BYTE_CAP bytes, decoding as UTF-8.
 * Marks truncated=true if the response contains more bytes than the cap.
 *
 * Falls back to `await response.text()` when `response.body` is undefined
 * (some test environments don't expose the stream); in that case we trust the
 * HEAD Content-Length check that already filtered >512KB upstream.
 */
export async function fetchText(
  response: Response,
  signal: AbortSignal
): Promise<{ text: string; truncated: boolean }> {
  if (!response.body) {
    const text = await response.text()
    return { text, truncated: false }
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8')
  const chunks: string[] = []
  let total = 0
  let truncated = false

  try {
    while (total < PREVIEW_BYTE_CAP) {
      if (signal.aborted) {
        // Throw an AbortError-shaped object so callers can distinguish.
        throw new DOMException('Aborted', 'AbortError')
      }
      const { done, value } = await reader.read()
      if (done) break
      const remaining = PREVIEW_BYTE_CAP - total
      if (value.byteLength > remaining) {
        const slice = value.subarray(0, remaining)
        chunks.push(decoder.decode(slice, { stream: true }))
        total += slice.byteLength
        truncated = true
        break
      }
      chunks.push(decoder.decode(value, { stream: true }))
      total += value.byteLength
    }
    // Flush the decoder.
    chunks.push(decoder.decode())
  } finally {
    // Make sure we release the reader whether we finished or hit the cap.
    try {
      await reader.cancel()
    } catch {
      // ignore
    }
  }

  return { text: chunks.join(''), truncated }
}

export default function CwdFilePreview(props: Props) {
  const { styles } = useStyles()
  const { agentName, selectedFile } = props
  const [state, setState] = useState<PreviewState>(initialState)
  // Track the controller for the in-flight fetch so we can abort it when the
  // user clicks another file. Without this, slow responses can arrive after
  // the user has switched files and overwrite the wrong state.
  const abortRef = useRef<AbortController | null>(null)
  // Track the last object URL so we can revoke it on switch / unmount. Blob
  // URLs are not GC'd eagerly; without explicit revoke they leak.
  const objectUrlRef = useRef<string | null>(null)

  useEffect(() => {
    // Cancel any previous in-flight fetch before starting a new one. This
    // fires both on file change and on unmount (via the cleanup returned
    // below) so responses never outlive the file they were issued for.
    if (abortRef.current) {
      abortRef.current.abort()
      abortRef.current = null
    }
    // Revoke the previous blob URL, if any. Setting ref to null here is
    // safe because the effect will populate it again if needed.
    if (objectUrlRef.current) {
      URL.revokeObjectURL(objectUrlRef.current)
      objectUrlRef.current = null
    }

    if (!selectedFile) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- reset preview state when no file is selected; this is the sync-handoff counterpart of the in-flight fetch below
      setState(initialState)
      return
    }

    const controller = new AbortController()
    abortRef.current = controller

    const path = fullPath(selectedFile)
    setState({ ...initialState, loading: true, downloadName: selectedFile.name })

    ;void (async () => {
      try {
        // HEAD precheck: we want Content-Length and Content-Type without
        // pulling the body. If the file is over the cap we render the
        // "too large" branch and never issue GET.
        let headLen: number | null = null
        let headMime = selectedFile.mime
        let headDisposition: string | null = null
        try {
          const head = await agentFilesApi.head(agentName, path)
          const len = head.headers.get('content-length')
          if (len) headLen = parseInt(len, 10)
          const ct = head.headers.get('content-type')
          if (ct) headMime = ct.split(';')[0].trim()
          headDisposition = head.headers.get('content-disposition')
        } catch {
          // HEAD failures are not fatal — fall through to GET path.
        }

        const downloadName =
          parseFilenameFromContentDisposition(headDisposition) ?? selectedFile.name

        if (headLen !== null && headLen > PREVIEW_BYTE_CAP) {
          setState({
            ...initialState,
            loading: false,
            kind: 'too-large',
            downloadName,
          })
          return
        }

        const kind = kindFromMime(headMime)

        const get = await agentFilesApi.getContent(agentName, path, {
          signal: controller.signal,
        })

        if (!get.ok) {
          setState({
            ...initialState,
            loading: false,
            error: `HTTP ${get.status}`,
            downloadName,
          })
          return
        }

        if (kind === 'image' || kind === 'pdf') {
          // Blob route — create object URL and revoke on switch/unmount.
          const blob = await get.blob()
          if (controller.signal.aborted) return
          const url = URL.createObjectURL(blob)
          objectUrlRef.current = url
          setState({
            ...initialState,
            loading: false,
            objectUrl: url,
            kind,
            downloadName,
          })
          return
        }

        if (kind === 'binary') {
          setState({
            ...initialState,
            loading: false,
            kind: 'binary',
            mime: headMime ?? null,
            downloadName,
          })
          return
        }

        // Text route (also used for text/html — content is rendered as escaped
        // text, never as live DOM).
        const { text, truncated } = await fetchText(get, controller.signal)
        if (controller.signal.aborted) return
        setState({
          ...initialState,
          loading: false,
          text: prettifyIfJson(selectedFile, text),
          truncated,
          kind: 'text',
          downloadName,
        })
      } catch (err) {
        if (controller.signal.aborted) return
        const msg = err instanceof Error ? err.message : String(err)
        if (msg === 'Aborted' || (err instanceof Error && err.name === 'AbortError')) return
        setState({
          ...initialState,
          loading: false,
          error: msg,
          downloadName: selectedFile.name,
        })
      }
    })()

    return () => {
      if (abortRef.current === controller) {
        controller.abort()
        abortRef.current = null
      }
    }
  }, [agentName, selectedFile])

  // Final unmount cleanup — belt-and-suspenders alongside the per-switch
  // revoke in the effect body above.
  useEffect(() => {
    return () => {
      if (objectUrlRef.current) {
        URL.revokeObjectURL(objectUrlRef.current)
        objectUrlRef.current = null
      }
    }
  }, [])

  if (!selectedFile) {
    return (
      <div className={styles.root}>
        <div className={styles.placeholder}>选择文件预览</div>
      </div>
    )
  }

  // Fetch the file with Authorization header, convert to blob URL, then
  // trigger a synthetic <a download> click. Browser-native <a href> would
  // issue an unauthenticated navigation request and 401 with "Needs
  // authorization". Going through fetch lets us attach the bearer token
  // (the axios interceptor doesn't run for <a> navigation).
  const handleDownload = async () => {
    try {
      const res = await agentFilesApi.getContent(agentName, fullPath(selectedFile))
      if (!res.ok) {
        message.error(`下载失败：HTTP ${res.status}`)
        return
      }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = state.downloadName ?? selectedFile.name
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch (err) {
      message.error(`下载失败：${parseApiError(err)}`)
    }
  }

  return (
    <div className={styles.root} data-testid="cwd-file-preview">
      <div className={styles.header}>
        <span className={styles.filename} title={selectedFile.name}>
          {selectedFile.name}
        </span>
        <button
          type="button"
          className={styles.downloadLink}
          onClick={handleDownload}
          aria-label="下载"
        >
          <DownloadIcon size={14} />
          下载
        </button>
      </div>

      {state.loading && <div className={styles.notice}>加载中…</div>}

      {!state.loading && state.error && (
        <div className={styles.notice}>
          <WarningIcon size={24} className={styles.noticeIcon} />
          <span className={styles.noticeText}>加载失败：{state.error}</span>
        </div>
      )}

      {!state.loading && state.kind === 'too-large' && (
        <div className={styles.notice}>
          <WarningIcon size={24} className={styles.noticeIcon} />
          <span className={styles.noticeText}>
            文件较大（超过 {(PREVIEW_BYTE_CAP / 1024).toFixed(0)} KB），仅提供下载。
          </span>
        </div>
      )}

      {!state.loading && state.truncated && (
        <div className={styles.truncated}>
          已截断：仅显示前 {(PREVIEW_BYTE_CAP / 1024).toFixed(0)} KB。完整内容请下载。
        </div>
      )}

      {!state.loading && state.kind === 'image' && state.objectUrl && (
        <img className={styles.embedded} src={state.objectUrl} alt={selectedFile.name} />
      )}

      {!state.loading && state.kind === 'pdf' && state.objectUrl && (
        <object
          className={styles.pdfObject}
          type="application/pdf"
          data={state.objectUrl}
          aria-label={selectedFile.name}
        >
          <div className={styles.notice}>
            <WarningIcon size={24} className={styles.noticeIcon} />
            <span className={styles.noticeText}>PDF 预览不可用，请使用下载按钮</span>
          </div>
        </object>
      )}

      {!state.loading && state.kind === 'text' && state.text !== null && (
        <div className={styles.body}>
          {/* text content rendered as escaped text — never dangerouslySetInnerHTML */}
          <pre className={styles.pre}>{state.text}</pre>
        </div>
      )}

      {!state.loading && state.kind === 'binary' && (
        <div className={styles.notice}>
          <WarningIcon size={24} className={styles.noticeIcon} />
          <span className={styles.noticeText}>
            不支持预览{state.mime ? `（${state.mime}）` : ''}
          </span>
        </div>
      )}
    </div>
  )
}
