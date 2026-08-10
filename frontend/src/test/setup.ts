import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

// Unmount React components after each test to avoid leaks.
// Then wait for any pending React scheduler macrotasks (flushPassiveEffects
// callbacks scheduled via MessageChannel) to complete before jsdom tears
// down the environment. Without this wait, React 19's scheduler may
// access `window.event` after jsdom is gone, producing:
//   ReferenceError: window is not defined
//   react-dom-client.development.js:17920 → schedulerEvent = window.event
// The wait is ~0ms (just yields to the macrotask queue) but eliminates
// the race that required CI reruns ~30% of the time.
afterEach(async () => {
  cleanup()
  await new Promise((resolve) => setTimeout(resolve, 0))
})

// Node >=22 ships an experimental `localStorage` global. Its state varies by
// Node version and flag combination:
//   - Node 22 + no flag: `globalThis.localStorage` is `undefined`
//   - Node 25+ no flag: `globalThis.localStorage` is an empty object `{}` with
//     NO Storage methods — accessing `.getItem` etc. throws TypeError
//   - Node + `--localstorage-file=<path>`: a working Storage
// vitest's jsdom integration may also clobber `window.localStorage`. To keep
// components that read/write localStorage during render (CwdFilePanel, theme
// store, sidebar state, etc.) working in tests, install an in-memory Storage
// shim whenever the existing binding is missing OR present-but-unusable.
class InMemoryStorage implements Storage {
  private store = new Map<string, string>()
  get length() {
    return this.store.size
  }
  clear() {
    this.store.clear()
  }
  getItem(key: string): string | null {
    return this.store.has(key) ? this.store.get(key)! : null
  }
  key(index: number): string | null {
    return Array.from(this.store.keys())[index] ?? null
  }
  removeItem(key: string) {
    this.store.delete(key)
  }
  setItem(key: string, value: string) {
    this.store.set(key, value)
  }
}

function isUsableStorage(v: unknown): v is Storage {
  if (v == null) return false
  const s = v as Partial<Storage>
  return (
    typeof s.getItem === 'function' &&
    typeof s.setItem === 'function' &&
    typeof s.removeItem === 'function' &&
    typeof s.clear === 'function'
  )
}

if (!isUsableStorage(globalThis.localStorage)) {
  const shim = new InMemoryStorage()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    writable: true,
    value: shim,
  })
  // jsdom sets globalThis === window in this environment, but be defensive in
  // case a future vitest upgrade separates them.
  if (typeof window !== 'undefined' && window !== globalThis) {
    ;(window as any).localStorage = shim
  }
}

// jsdom does not implement matchMedia; antd / lobe-ui expect it.
// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime defense: jsdom does not implement matchMedia despite TS lib typing it as required
if (!window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn()
    })
  })
}

// jsdom lacks ResizeObserver; antd uses it for layout measurement.
// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- runtime defense: jsdom does not implement ResizeObserver despite TS lib typing it as required
if (!window.ResizeObserver) {
  window.ResizeObserver = class {
    observe(): void { /* no-op: jsdom stub */ }
    unobserve(): void { /* no-op: jsdom stub */ }
    disconnect(): void { /* no-op: jsdom stub */ }
  }
}

// jsdom does not implement getComputedStyle(elt, pseudoElt) — it throws
// "Not implemented: window.getComputedStyle(elt, pseudoElt)" when antd /
// @ant-design/cssinjs query pseudo-elements (::before, ::after) for style
// injection. Strip the pseudoElt argument and fall back to the element-only
// call so component effects don't blow up mid-render.
const origGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = ((elt: Element, pseudoElt?: string | null) => {
  if (pseudoElt) return origGetComputedStyle(elt)
  return origGetComputedStyle(elt)
})

// jsdom does not implement URL.createObjectURL / revokeObjectURL. Components
// that produce blob: URLs for image/PDF previews (and tests that exercise
// them) need stubs that return deterministic URLs. We track issued URLs so
// tests can assert revoke behavior.
if (!('createObjectURL' in URL) || typeof URL.createObjectURL !== 'function') {
  let counter = 0
  const issued = new Set<string>()
  URL.createObjectURL = ((_blob: Blob) => {
    counter++
    const url = `blob:mock/${counter}`
    issued.add(url)
    return url
  })
  URL.revokeObjectURL = ((url: string) => {
    issued.delete(url)
  })
}
