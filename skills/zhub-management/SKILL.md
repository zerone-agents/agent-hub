---
name: zhub-management
description: |
  Manage Zerone Hub resources via the `zhub` CLI. Use when user asks to
  create/update/delete agents, skills, tools, mcps, providers. Covers full CRUD on
  five domains: Agent, Skill, Tool, MCP, and Provider+Model.
---

# Zerone Hub Management

## Overview

This skill teaches you how to manage Zerone Hub resources through the `zhub` CLI.

## When to Use

Trigger when the user asks to manage resources on Zerone Hub:

- Creating, updating, or deleting agents
- Deploying, starting, stopping, or undeploying agents
- Uploading or deleting skills
- Creating, updating, or deleting tools and MCPs
- Creating, updating, or deleting providers and models
- Listing agents, skills, tools, MCPs, or providers
- Checking CLI login status or troubleshooting CLI access

**Do NOT use when:** the user is asking about the control-panel web UI, runtime APIs, or frontend behavior that does not go through `zhub`.

## Supported resource domains

- **Agent** (CRUD, deploy lifecycle, associations)
- **Skill** (upload, download, delete)
- **Tool** (CRUD, default flag)
- **MCP** (CRUD, headers are masked in CLI output)
- **Provider + Model** (CRUD, probe)

## Pre-flight Check

Every time you enter this skill, first verify the CLI is ready:

1. **Check installation**: Run `which zhub`. If not found, tell the user:
   ```
   npm install -g @zerone-agent/zhub
   ```
   Or with Bun: `bun add -g @zerone-agent/zhub`

2. **Check login**: Run `zhub whoami`. If it fails or shows no profile, tell the user:
   ```
   zhub login --url <server-url> --token cli_xxxxxxxxxxxx
   ```
   The token can be obtained from control-panel web UI: Settings → CLI Tokens.

## Output Convention

**Always use `--output json`** when you need to parse the output programmatically:

```bash
zhub agent list --output json     # parseable JSON with { data, meta } wrapper
zhub agent get my-agent --output json
```

Default `table` output is for humans — it contains formatting characters that are hard to parse.

For YAML output (single objects): `--output yaml`

## Decision Tree

### 3.1 "Create an agent"

1. List existing agents to understand naming conventions:
   ```bash
   zhub agent list --output json
   ```
2. List available providers and models:
   ```bash
   zhub provider list --output json
   zhub provider get <id> --output json   # see available models
   ```
3. Ask the user for required fields:
   - **id** (required, kebab-case identifier, e.g. `research-agent`)
   - **config.title** (localized display titles, e.g. `zh` and `en`)
   - **config.description** (localized descriptions, e.g. `zh` and `en`)
   - **config.modelId** (modelId from provider, e.g. `claude-sonnet-4-5`)
   - **config.systemPrompt** (the agent's system prompt)
4. Write an `agent.yaml` file:
   ```yaml
   id: research-agent
   desktop: true
   mobile: false
   isDefault: false
   config:
     title:
       zh: 研究员
       en: Research Agent
     description:
       zh: 负责深度研究与总结
       en: Performs deep research and synthesis
     modelId: claude-sonnet-4-5
     systemPrompt: |
       你是一个研究员，擅长深度搜索和总结。
     maxTurns: 20
     permissionMode: auto
   ```
5. Create the agent:
   ```bash
   zhub agent create --file ./agent.yaml --output json
   ```
6. Verify:
   ```bash
   zhub agent get research-agent --output json
   ```
7. Optionally deploy:
   ```bash
   zhub agent deploy research-agent
   ```

### 3.2 "Upload a skill"

1. Verify the skill directory has `SKILL.md` with valid frontmatter (`name` + `description`).
2. Ask the user for the skill name (kebab-case, globally unique) if not obvious from frontmatter.
3. Pack and upload:
   ```bash
   zhub skill create --from-dir ./my-skill/ \
     --title "中文名称" \
     --title-en "English title" \
     --description "中文描述" \
     --description-en "English description" \
     --output json
   ```
   The CLI auto-reads `name` and falls back to the `SKILL.md` frontmatter
   description when `--description` is omitted. The four metadata flags
   `--title`, `--title-en`, `--description`, and `--description-en` are
   available on both `skill create` and `skill update`.
4. Verify:
   ```bash
   zhub skill get <skill-name>
   ```

### 3.3 "Test a provider"

**Mode A — test a stored provider:**
```bash
zhub provider probe <id>
```

**Mode B — test a custom config (not yet saved):**
```bash
zhub provider probe \
  --base-url https://api.anthropic.com \
  --api-key sk-ant-xxxx \
  --protocol anthropic \
  --output json
```

If the probe succeeds, suggest creating the provider:
```bash
zhub provider create --file ./provider.yaml
```

### 3.4 "Create or update a tool"

1. Ask the user for:
   - **name** (required, kebab-case identifier, e.g. `my-tool`)
   - **title** (required, display title, e.g. `我的工具`)
   - **description** (required, what the tool does)
   - **isDefault** (optional boolean, defaults to false)
2. Write a `tool.yaml` file:
   ```yaml
   name: my-tool
   title: 我的工具
   description: 这是一个示例工具
   isDefault: false
   ```
3. Create or update:
   ```bash
   zhub tool create --file ./tool.yaml --output json
   zhub tool update my-tool --file ./tool.yaml --output json
   ```
4. Verify:
   ```bash
   zhub tool get my-tool --output json
   ```

### 3.5 "Create or update an MCP"

1. Ask the user for:
   - **name** (required, kebab-case identifier)
   - **title** (required, display title)
   - **description** (required)
   - **transportType** (required, `sse` or `http`)
   - **url** (optional, e.g. `http://localhost:3001/sse`)
   - **headers** (optional key-value map, e.g. `Authorization`, `X-Api-Key`)
   - **retryMaxRetries** / **retryTimeoutMs** (optional)
2. Write an `mcp.yaml` file:
   ```yaml
   name: my-mcp
   title: 我的 MCP
   description: 这是一个示例 MCP 服务
   transportType: http
   url: http://localhost:3001/sse
   headers:
     Authorization: Bearer xxxx
     X-Api-Key: sk-xxxx
   retryMaxRetries: 3
   retryTimeoutMs: 5000
   ```
3. 探测 MCP 以验证配置并获取 tools：
   ```bash
   zhub mcp probe --file ./mcp.yaml --output json
   ```
4. 探测成功后创建（CLI 会自动探测，也可以手动）：
   ```bash
   zhub mcp create --file ./mcp.yaml --output json
   zhub mcp update my-mcp --file ./mcp.yaml --output json
   ```
   注：SSE transport 暂不支持探测，创建时不强制。
5. Verify:
   ```bash
   zhub mcp get my-mcp --output json
   ```
   Headers will be masked as `<hidden>` in the output.

### 3.6 "Update agent associations"

Use dedicated association commands (not `update`, which overwrites the full config):

```bash
# Set subagents
zhub agent set-subagents <name> subagent-a subagent-b

# Set tools
zhub agent set-tools <name> bash edit read write

# Set mcps
zhub agent set-mcps <name> mcp-a mcp-b

# Set skills
zhub agent set-skills <name> skill-1 skill-2
```

### 3.7 "Deploy lifecycle"

```bash
zhub agent deploy <name>          # deploy (creates runtime container)
zhub agent status <name>          # check deployment status
zhub agent stop <name>            # stop running container
zhub agent start <name>           # restart stopped container
zhub agent undeploy <name>        # archive (keeps data)
zhub agent undeploy <name> --purge  # permanently delete
```

## Common Mistakes

- **MCP headers are masked**: `zhub mcp get` always shows headers as `<hidden>`; you cannot read back the original secret after creation. If lost, update the MCP with new headers.
- **Provider apiKey is write-only**: After creating a provider, the API key cannot be read back. If lost, `update` the provider with a new key.
- **Deleting an agent doesn't auto-undeploy**: Always `zhub agent undeploy <name>` before `zhub agent delete <name>`.
- **Skill zip excludes hidden dirs**: `.git`, `node_modules`, `dist`, `build` are automatically excluded from the zip.
- **Skill name must be globally unique**: Check with `zhub skill list` before creating.
- **Agent YAML has two supported shapes**: `id` is required when creating an agent and optional when updating one. Prefer native `config` with `title`, `description`, `modelId`, and other configuration nested beneath it. The compatible flat shape accepts `title`, `description`, `model`, `systemPrompt`, `maxTurns`, and `permissionMode`; `model` maps to `config.modelId`, and `name` remains a legacy Chinese-title alias. Do not define the same normalized field in flat and nested form because the CLI rejects conflicts. See `references/agent.yaml`.
- **Agent update ID must match when provided**: In `zhub agent update <name> --file agent.yaml`, an optional YAML `id` must exactly match positional `<name>`. Agent update accepts `--file`, not `--json`.
- **Agent platform flags**: On create, `--desktop`/`--no-desktop` and `--mobile`/`--no-mobile` override YAML `desktop`/`mobile`; both default to false when omitted. `--default`/`--no-default` overrides YAML `isDefault`, with native last-argument-wins behavior. Update takes state from YAML and leaves omitted state fields unchanged.
- **Using `update` to change associations**: `update` overwrites the whole config. Use `set-subagents`, `set-tools`, or `set-skills` instead.
- **Tool/MCP names are URL-encoded internally**: Use the raw name in CLI commands, even with spaces or special characters.
- **MCP probe is required for create**: CLI automatically probes before creating. If probe fails, creation is aborted. SSE transport is not supported for probe yet.

## Error Recovery

| Exit code | HTTP status | Meaning | Action |
|-----------|-------------|---------|--------|
| 0 | 200 | Success | — |
| 2 | 400 | Bad request | Check parameters, fix and retry |
| 3 | 401 | Unauthorized | Tell user to `zhub login` again |
| 4 | 403 | Forbidden | User needs admin role in control-panel |
| 5 | 404 | Not found | Check name/id exists |
| 6 | 409 | Conflict | Resource already exists, use different name or `update` |
| 8 | 5xx | Server error | Retry once, then report to user |
| 9 | — | Network error | Check server URL and connectivity |

## Command Reference

### Global
```
zhub login --url <url> --token <token>   # Authenticate
zhub whoami                               # Show current user
zhub --help                               # Full help
```

### Agent (14 commands)
```
zhub agent list [--desktop] [--mobile] [--output json|table|yaml]
zhub agent get <name> [--output yaml]
zhub agent create --file <path> [--desktop|--no-desktop] [--mobile|--no-mobile] [--default|--no-default] [--output yaml]
zhub agent update <name> --file <path> [--output yaml]
zhub agent delete <name>
zhub agent set-subagents <name> <sub1> [sub2 ...]
zhub agent set-tools <name> <tool1> [tool2 ...]
zhub agent set-mcps <name> <mcp1> [mcp2 ...]
zhub agent set-skills <name> <skill1> [skill2 ...]
zhub agent deploy <name> [--force]
zhub agent undeploy <name> [--purge]
zhub agent start <name>
zhub agent stop <name>
zhub agent status <name> [--output json|text]
```

### Provider (6 commands)
```
zhub provider list [--output json|table|yaml]
zhub provider get <id> [--output yaml]
zhub provider create --file <path> | --json '<json>'
zhub provider update <id> --file <path> | --json '<json>'
zhub provider delete <id>
zhub provider probe <id> | --base-url X --api-key Y --protocol Z
```

### Skill (6 commands)
```
zhub skill list [--type expert|community] [--output json|table|yaml]
zhub skill get <name> [--output yaml]
zhub skill create --from-dir <dir> [--name N] [--title T] [--title-en T] [--description D] [--description-en D] [--type T]
zhub skill update <name> --from-dir <dir> [--title T] [--title-en T] [--description D] [--description-en D] [--type T]
zhub skill delete <name>
zhub skill download <name> [--output json|text]
```

### Tool (5 commands)
```
zhub tool list [--output json|table|yaml]
zhub tool get <name> [--output yaml]
zhub tool create --file <path> | --json '<json>' [--output json|table|yaml]
zhub tool update <name> --file <path> | --json '<json>' [--output json|table|yaml]
zhub tool delete <name>
```

### MCP (6 commands)
```
zhub mcp list [--output json|table|yaml]
zhub mcp get <name> [--output yaml]
zhub mcp create --file <path> | --json '<json>' [--output json|table|yaml]
zhub mcp update <name> --file <path> | --json '<json>' [--output json|table|yaml]
zhub mcp delete <name>
zhub mcp probe <name> | --file <path> | --json '<json>' [--output json|table|yaml]
```
