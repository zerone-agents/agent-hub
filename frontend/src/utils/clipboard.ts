import { showManualCopy } from '@/components/ManualCopyDialog'

/**
 * Copy text to the clipboard with a legacy fallback.
 *
 * The async Clipboard API is only exposed in secure contexts (HTTPS or
 * localhost). On plain-HTTP remote deployments (e.g. testing over
 * http://<ip>:<port>) navigator.clipboard is undefined, so we fall back to
 * the deprecated-but-universal document.execCommand('copy') route.
 *
 * IMPORTANT: on modern Chromium in insecure contexts execCommand('copy')
 * silently no-ops while still returning true, and clipboard contents cannot
 * be read back for verification — use copyOrManual() for user-facing actions
 * so a manual-copy dialog always backs the best-effort path.
 *
 * The fallback follows the clipboard.js recipe: a readonly textarea pinned
 * off-viewport (display:none would make it unselectable; opacity:0 alone is
 * rejected by some WebKit versions), focused and explicitly selection-ranged
 * before issuing the copy command.
 *
 * Returns whether the copy succeeded so callers can surface feedback —
 * silent failure leaves testers guessing on plain-HTTP test deployments.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime defense: clipboard API may be undefined in non-HTTPS / older browsers despite TS lib typing it as required
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // permission denied or unsupported MIME — try the legacy path too
    }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.left = '-9999px'
    ta.style.top = '0'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    ta.setSelectionRange(0, text.length) // Safari/mobile: select() alone is unreliable
    try {
      // eslint-disable-next-line @typescript-eslint/no-deprecated -- only programmatic fallback available for non-secure (plain HTTP) contexts where navigator.clipboard is undefined
      return document.execCommand('copy')
    } finally {
      document.body.removeChild(ta) // guaranteed even when execCommand is unavailable (throws)
    }
  } catch {
    return false
  }
}

/**
 * User-facing copy: secure contexts use copyToClipboard() and return its real
 * result; insecure contexts (plain HTTP + IP) run a best-effort execCommand
 * and ALWAYS open the manual-copy dialog, because modern Chromium silently
 * no-ops execCommand there (returns true without writing) and the clipboard
 * cannot be read back to verify — the dialog is the only dependable path.
 */
export async function copyOrManual(text: string): Promise<boolean> {
  if (window.isSecureContext) {
    return copyToClipboard(text)
  }
  void copyToClipboard(text) // best-effort: Safari / Firefox may genuinely copy
  showManualCopy(text)
  return true // handled via the manual dialog
}
