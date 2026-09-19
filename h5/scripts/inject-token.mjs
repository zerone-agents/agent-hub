#!/usr/bin/env node
/**
 * inject-token.mjs — 把线上 console 的登录态搬移到本地 H5 dev（agenthub_h5 适配版）。
 *
 * 背景：
 *   线上 console.zerone.life 是 Casdoor SSO 模式，没有密码登录接口，本地 dev
 *   无法完成 OAuth 回调。但线上签发的 JWT 对 localhost 同样有效——只要完整
 *   token 进入 localStorage。手动 F12 复制经 shell/剪贴板容易截断 RSA 签名，
 *   本脚本通过 Vite public/ 下的临时文件中转，无长度限制。
 *
 * 与桌面 console 版（agent-hub/frontend/scripts/inject-token.mjs）的差异：
 *   - dev 目标 = http://localhost:3000（Express + Vite middleware，public/ 挂根路径）
 *   - 注入 key = zerone_auth（H5 的 JSON 登录态），不是 access_token/refresh_token
 *   - 注入时调 /auth/userinfo 补全 username/role，拼好完整 AuthState
 *   - 验证端点 = /api/v1/agents?view=chat（对客视图，guest 也在白名单）
 *
 * 前置条件：
 *   - opencli 已安装（npm i -g @jackwener/opencli）且 `opencli doctor` 全绿（Chrome 扩展已连接）
 *   - Chrome 已登录 https://console.zerone.life
 *   - H5 dev server 在 http://localhost:3000 运行（AGENT_HUB_URL 指向线上）
 *
 * 用法：
 *   npm run inject-token            # 默认参数
 *   node scripts/inject-token.mjs --help
 *
 * 副作用：
 *   - 临时创建 public/_token.json（成功/失败都会删除）
 *   - 通过 opencli 打开两个浏览器页面（读线上、写本地）
 *
 * 退出码：0 成功；1 失败（带诊断信息）。
 */

import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const H5_ROOT = resolve(__dirname, '..')

// --- 配置（可用 flag 覆盖） -------------------------------------------------
const PROD_ORIGIN = 'https://console.zerone.life'
const DEV_ORIGIN = 'http://localhost:3000'
const PUBLIC_TOKEN_PATH = resolve(H5_ROOT, 'public', '_token.json')
const SESSION = 'h5-token-sync'

// --- 参数解析 ---------------------------------------------------------------
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

const HELP = `inject-token.mjs — 把线上登录态搬到本地 H5 dev

用法:
  node scripts/inject-token.mjs [options]

选项:
  --prod-origin <url>   线上 console 地址 (默认: ${PROD_ORIGIN})
  --dev-origin <url>    本地 dev 地址     (默认: ${DEV_ORIGIN})
  -h, --help            显示帮助

前置条件:
  - opencli 已安装且浏览器桥已连接（先跑: opencli doctor）
  - Chrome 已登录线上 console
  - H5 dev server 运行中（npm run dev，AGENT_HUB_URL 指向线上）

流程:
  1. opencli 读线上 localStorage 的 access_token + refresh_token
  2. 写入 public/_token.json（由 Vite middleware 提供服务）
  3. 本地 dev 页 fetch 该文件，调 /auth/userinfo 拼好 zerone_auth 写入 localStorage
  4. 用 /api/v1/agents?view=chat 验证，然后清理临时文件
`

function fail(msg, code = 1) {
  console.error(`✖ ${msg}`)
  process.exit(code)
}

// --- opencli 辅助 ------------------------------------------------------------
function opencli(...args) {
  const r = spawnSync('opencli', args, { encoding: 'utf8' })
  if (r.status !== 0) {
    const stderr = (r.stderr || '').trim()
    const stdout = (r.stdout || '').trim()
    throw new Error(
      `opencli ${args.join(' ')} exited with ${r.status}\n${stderr}\n${stdout}`.trim(),
    )
  }
  return r.stdout
}

function opencliEval(session, jsExpr) {
  return opencli('browser', session, 'eval', jsExpr)
}

// --- 主流程 -------------------------------------------------------------------
async function main() {
  const args = parseArgs(process.argv.slice(2))
  if (args.help) {
    process.stdout.write(HELP)
    return
  }

  // 预检：opencli 存在
  try {
    execFileSync('opencli', ['--version'], { stdio: 'ignore' })
  } catch {
    fail('opencli 未安装。安装: npm i -g @jackwener/opencli')
  }

  // 预检：浏览器桥已连接（doctor 无 JSON 输出，解析文本）
  const doctorText = opencli('doctor')
  if (!/\[OK\]\s+Connectivity:\s+connected/.test(doctorText)) {
    fail(
      'opencli 浏览器桥未连接。运行 `opencli doctor`，按提示安装 Chrome 扩展并 reload。',
    )
  }

  console.log('→ 打开线上 console 读取 token…')
  opencli('browser', SESSION, 'open', `${args.prodOrigin}/static/dashboard`)
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
    fail(`无法解析线上 token。原始输出:\n${tokenJson.slice(0, 400)}`)
  }

  if (!tokens.at || !tokens.rt) {
    fail(`${args.prodOrigin} 的 localStorage 里没有 token，请先在 Chrome 登录线上 console。`)
  }
  console.log(`  access_token:  ${tokens.at.length} chars`)
  console.log(`  refresh_token: ${tokens.rt.length} chars`)

  // 临时文件放 public/（Vite middleware 挂根路径，dev 页 fetch('/_token.json') 可取）
  if (!existsSync(resolve(H5_ROOT, 'public'))) {
    mkdirSync(resolve(H5_ROOT, 'public'), { recursive: true })
  }
  writeFileSync(PUBLIC_TOKEN_PATH, JSON.stringify(tokens))
  console.log(`→ 写入 ${PUBLIC_TOKEN_PATH.replace(H5_ROOT + '/', '')}`)

  try {
    console.log(`→ 打开 ${args.devOrigin} 注入登录态…`)
    opencli('browser', SESSION, 'open', args.devOrigin)
    await sleep(800)

    const verify = opencliEval(
      SESSION,
      `(async () => {
        try {
          const r = await fetch('/_token.json');
          if (!r.ok) return JSON.stringify({ err: 'fetch _token.json: HTTP ' + r.status });
          const { at, rt } = await r.json();

          // 调 /auth/userinfo 补全 username/role，拼 H5 的 AuthState
          const ui = await fetch('/auth/userinfo', {
            headers: { Authorization: 'Bearer ' + at },
          });
          if (!ui.ok) return JSON.stringify({ err: 'userinfo: HTTP ' + ui.status + '（token 可能已过期，回线上重新登录再跑）' });
          const body = await ui.json();
          const d = body.data ?? body;
          const roles = Array.isArray(d.roles) ? d.roles : [];
          const auth = {
            token: at,
            refreshToken: rt,
            username: d.display_name || d.username || 'Zerone 用户',
            role: roles[0],
          };
          localStorage.setItem('zerone_auth', JSON.stringify(auth));

          // 验证：对客 Agent 列表（guest 白名单内）
          const probe = await fetch('/api/v1/agents?view=chat', {
            headers: { Authorization: 'Bearer ' + at },
          });
          return JSON.stringify({
            username: auth.username,
            role: auth.role || '(无角色)',
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
      fail(`注入步骤返回非 JSON。原始输出:\n${verify.slice(0, 400)}`)
    }

    if (result.err) fail(`注入失败: ${result.err}`)
    if (result.api_status !== 200) {
      fail(
        `登录态已注入但 agents 接口返回 HTTP ${result.api_status}。响应: ${result.api_body_head}`,
      )
    }

    console.log(
      `  注入完成: ${result.username}（${result.role}），/api/v1/agents?view=chat → ${result.api_status}`,
    )
    console.log('→ 刷新页面使登录态生效…')
    opencli('browser', SESSION, 'open', args.devOrigin)
  } finally {
    // 无论成败都删掉临时 token 文件，不把凭证留在 public/
    if (existsSync(PUBLIC_TOKEN_PATH)) {
      rmSync(PUBLIC_TOKEN_PATH, { force: true })
      console.log(`→ 已删除 ${PUBLIC_TOKEN_PATH.replace(H5_ROOT + '/', '')}`)
    }
  }

  console.log('\n✓ 登录态注入完成 —— 本地 H5 现在就是线上登录身份，刷新即可。')
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms))
}

main().catch((e) => fail(e?.stack || e?.message || String(e)))
