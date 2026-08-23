/**
 * Deterministic backstop for the flaky "ReferenceError: window is not defined"
 * CI failure in frontend-test / frontend-test-full.
 *
 * Root-cause chain: React 19's commit phase schedules passive-effect flushing
 * through the Scheduler, which in Node uses `setImmediate`. The scheduled
 * callback reads the bare global `window`
 * (react-dom-client.development.js: `schedulerEvent = window.event`). If that
 * immediate fires AFTER vitest has torn down the per-file jsdom environment
 * (e.g. a component's real long timer — antd animation ~500ms — fires after
 * the afterEach drain, which is millisecond-scale), `window` no longer exists
 * and the read throws; vitest records it as an unhandled error and fails the
 * run even though all tests passed.
 *
 * Upstream status: still unguarded in react-dom 19.2.8 (latest on npm at the
 * time of writing). Re-check when upgrading react-dom — once upstream guards
 * the `window.event` read, this predicate and the listener wrapping in
 * setup.ts can be removed.
 *
 * The predicate is deliberately strict (all three conditions must hold) so it
 * can never swallow a genuine product regression.
 */

export function isReactTeardownFlake(err: unknown): boolean {
  // 1. Must be a ReferenceError (by instanceof or name, to survive realm/clone edge cases).
  if (!(err instanceof ReferenceError || (err instanceof Error && err.name === 'ReferenceError'))) {
    return false
  }
  // 2. Exact message match — no substring matching, so no other ReferenceError slips through.
  if (err.message !== 'window is not defined') {
    return false
  }
  // 3. jsdom must be gone: this error is only possible AFTER teardown. While
  //    `window` exists, product code can never produce this message, so a hit
  //    here proves the flake path (scheduler immediate racing env teardown).
  if (typeof globalThis.window !== 'undefined') {
    return false
  }
  return true
}
