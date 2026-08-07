#!/usr/bin/env node
/**
 * inject-token.mjs — Copy login session from production console to local dev.
 *
 * Why this exists:
 *   Vite dev server proxies /api and /auth to https://console.zerone.life,
 *   so the backend is shared. The JWT issued by the shared Casdoor works
 *   against localhost too — but only if the full token reaches localStorage.
 *   Pasting it via `opencli browser eval` truncates the string on the way
 *   through the shell (RSA signature then fails with "verification error").
 *
 *   This script routes the token through a temp file served by Vite's
 *   public/ dir, which avoids any command-line length limit.
 *
 * Prerequisites:
 *   - opencli installed (npm i -g @jackwener/opencli) and `opencli doctor` green
 *   - Chrome logged into https://console.zerone.life
 *   - Vite dev server running on http://localhost:7002
 *
 * Usage:
 *   node scripts/inject-token.mjs            # default ports/URLs
 *   node scripts/inject-token.mjs --help     # see all flags
 *
 * Side effects:
 *   - Creates frontend/public/_token.json (auto-deleted on success)
 *   - Opens two browser sessions via opencli (reads prod, writes dev)
 *
 * Exit codes: 0 on success, 1 on any failure (with diagnostic message).
 */

import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const FRONTEND_ROOT = resolve(__dirname, '..')

// --- Config (overridable via flags) ---------------------------------------
const PROD_ORIGIN = 'https://console.zerone.life'
const DEV_ORIGIN = 'http://localhost:7002'
const DEV_LOGIN_PATH = '/static/login'
const DEV_DASHBOARD_PATH = '/static/dashboard'
const PUBLIC_TOKEN_PATH = resolve(FRONTEND_ROOT, 'public', '_token.json')
const SESSION = 'token-sync'

// --- arg parsing ----------------------------------------------------------
function parseArgs(argv) {
  const args = { prodOrigin: PROD_ORIGIN, devOrigin: DEV_ORIGIN, help: false }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    if (a === '--help' || a === '-h') args.help = true
    else if (a === '--prod-origin') args.prodOrigin = argv[++i]
    else if (a === '--dev-origin') args.devOrigin = argv[++i]
    else if (a.startsWith('--prod-origin=')) args.prodOrigin = a.slice('--prod-origin='.length)
    else if (a.startsWith('--dev-origin=')) args.devOrigin = a.slice('--dev-origin='.length)
  }
  return args
}

const HELP = `inject-token.mjs — copy login from prod console to local dev

Usage:
  node scripts/inject-token.mjs [options]

Options:
  --prod-origin <url>   Production console origin (default: ${PROD_ORIGIN})
  --dev-origin <url>    Local dev origin      (default: ${DEV_ORIGIN})
  -h, --help            Show this help

Prerequisites:
  - opencli installed + Chrome extension connected (run: opencli doctor)
  - Chrome logged into the production console
  - Vite dev server running on the dev origin

What it does:
  1. Reads access_token + refresh_token from prod localStorage via opencli.
  2. Writes them to frontend/public/_token.json (served by Vite).
  3. Navigates the local dev tab to fetch that file and populate localStorage.
  4. Verifies with a real /api/v1/admin/agents call, then cleans up.
`

function fail(msg, code = 1) {
  console.error(`✖ ${msg}`)
  process.exit(code)
}

// --- opencli helper -------------------------------------------------------
// Returns stdout (string). Throws on non-zero exit.
function opencli(...args) {
  const r = spawnSync('opencli', args, { encoding: 'utf8' })
  if (r.status !== 0) {
    const stderr = (r.stderr || '').trim()
    const stdout = (r.stdout || '').trim()
    throw new Error(
      `opencli ${args.join(' ')} exited with ${r.status}\n${stderr}\n${stdout}`.trim(),
    )
  }
  // opencli may print status/warning lines on stderr; stdout is the payload.
  return r.stdout
}

function opencliJson(...args) {
  const out = opencli(...args)
  try {
    return JSON.parse(out)
  } catch (e) {
    throw new Error(`opencli ${args.join(' ')} returned non-JSON:\n${out.slice(0, 400)}`)
  }
}

function opencliEval(session, jsExpr) {
  // eval output is the raw JS return value printed as JSON by opencli.
  return opencli('browser', session, 'eval', jsExpr)
}

// --- main -----------------------------------------------------------------
async function main() {
  const args = parseArgs(process.argv.slice(2))
  if (args.help) {
    process.stdout.write(HELP)
    return
  }

  // Preflight: opencli present.
  try {
    execFileSync('opencli', ['--version'], { stdio: 'ignore' })
  } catch {
    fail('opencli not found. Install: npm i -g @jackwener/opencli')
  }

  // Connectivity probe: opencli doctor prints a human-readable report and
  // doesn't support -f json, so we parse the text output instead. The line
  // we care about is "[OK] Connectivity: connected in <ms>".
  const doctorText = opencli('doctor')
  if (!/\[OK\]\s+Connectivity:\s+connected/.test(doctorText)) {
    fail(
      'opencli browser bridge not connected. Run `opencli doctor`, install the Chrome extension from https://github.com/jackwener/opencli/releases, and reload it.',
    )
  }

  console.log('→ Opening production console to read tokens…')
  opencli('browser', SESSION, 'open', `${args.prodOrigin}/static/dashboard`)

  // Give SPA a moment to settle if it needs to hydrate.
  await sleep(800)

  const tokenJson = opencliEval(
    SESSION,
    `JSON.stringify({
      at: localStorage.getItem('access_token') || '',
      rt: localStorage.getItem('refresh_token') || '',
    })`,
  )

  let tokens
  try {
    tokens = JSON.parse(tokenJson)
  } catch {
    fail(`Could not parse token envelope from prod. Raw output:\n${tokenJson.slice(0, 400)}`)
  }

  if (!tokens.at || !tokens.rt) {
    fail(
      `No tokens in ${args.prodOrigin} localStorage. Sign in to the production console first.`,
    )
  }
  console.log(`  access_token:  ${tokens.at.length} chars`)
  console.log(`  refresh_token: ${tokens.rt.length} chars`)

  // Drop the token file inside Vite's public/ so the dev tab can fetch it.
  if (!existsSync(resolve(FRONTEND_ROOT, 'public'))) {
    mkdirSync(resolve(FRONTEND_ROOT, 'public'), { recursive: true })
  }
  writeFileSync(PUBLIC_TOKEN_PATH, JSON.stringify(tokens))
  console.log(`→ Wrote ${PUBLIC_TOKEN_PATH.replace(FRONTEND_ROOT + '/', '')}`)

  try {
    console.log(`→ Opening ${args.devOrigin} to inject tokens…`)
    opencli('browser', SESSION, 'open', `${args.devOrigin}${DEV_LOGIN_PATH}`)
    await sleep(500)

    const verify = opencliEval(
      SESSION,
      `(async () => {
        try {
          const r = await fetch('/static/_token.json');
          if (!r.ok) return JSON.stringify({ err: 'fetch _token.json: HTTP ' + r.status });
          const { at, rt } = await r.json();
          localStorage.setItem('access_token', at);
          localStorage.setItem('refresh_token', rt);
          const probe = await fetch('/api/v1/admin/agents?page=1', {
            headers: { Authorization: 'Bearer ' + at },
          });
          return JSON.stringify({
            at_len: localStorage.getItem('access_token').length,
            rt_len: localStorage.getItem('refresh_token').length,
            api_status: probe.status,
            api_body_head: (await probe.text()).slice(0, 120),
          });
        } catch (e) {
          return JSON.stringify({ err: String(e) });
        }
      })()`,
    )

    let result
    try {
      result = JSON.parse(verify)
    } catch {
      fail(`Inject step returned non-JSON. Raw:\n${verify.slice(0, 400)}`)
    }

    if (result.err) fail(`Injection failed: ${result.err}`)
    if (result.api_status !== 200) {
      fail(
        `Token injected but API returned HTTP ${result.api_status}. Body: ${result.api_body_head}`,
      )
    }

    console.log(
      `  injected: at=${result.at_len} chars, rt=${result.rt_len} chars, /api/v1/admin/agents → ${result.api_status}`,
    )

    console.log('→ Reloading dashboard to confirm end-to-end…')
    opencli('browser', SESSION, 'open', `${args.devOrigin}${DEV_DASHBOARD_PATH}`)
    await sleep(1200)

    const state = opencli('browser', SESSION, 'state')
    const onDashboard = state.includes(`${args.devOrigin}${DEV_DASHBOARD_PATH}`)
    const bouncedToLogin = state.includes(`${DEV_LOGIN_PATH}`)
    if (!onDashboard || bouncedToLogin) {
      fail(
        'Dashboard check failed — page bounced back to login. Check the app may clear tokens on some additional validation.',
      )
    }
    console.log('  still on dashboard ✓')
  } finally {
    // Always remove the token file — never leave creds lying around in public/.
    if (existsSync(PUBLIC_TOKEN_PATH)) {
      rmSync(PUBLIC_TOKEN_PATH, { force: true })
      console.log(`→ Removed ${PUBLIC_TOKEN_PATH.replace(FRONTEND_ROOT + '/', '')}`)
    }
  }

  console.log('\n✓ Tokens injected — local dev is now signed in as the prod user.')
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms))
}

main().catch((e) => fail(e?.stack || e?.message || String(e)))
