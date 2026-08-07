# zhub

Zerone Hub CLI — manage agents, skills, tools, MCPs, and providers from the terminal.
Designed for AI agent hosts (Claude Code, Cursor, Cline, etc.) to invoke via shell.

## Install

```bash
npm install -g @zerone-agent/zhub
```

Requires Node.js ≥ 18 or Bun ≥ 1.0.

## Login

```bash
# 1. Create a CLI token in control-panel web UI (User Menu → CLI Tokens)
# 2. Login (non-interactive, all flags required):
zhub login --url https://console.currantmind.cn --token cli_xxxxxxxxxxxx
```

## Command Overview

### Global
```bash
zhub whoami                    # show current profile + user
zhub --help                    # full help
```

### Agent (14 commands)
```bash
# CRUD
zhub agent list [--enabled] [--disabled] [--output json|table|yaml]
zhub agent get <name> [--output yaml]
zhub agent create --file <agent.yaml> [--enabled|--disabled] [--default|--no-default] [--output yaml]
zhub agent update <name> --file <agent.yaml> [--output yaml]
zhub agent delete <name>

# Relations
zhub agent set-subagents <name> <sub1> [sub2 ...]
zhub agent set-tools <name> <tool1> [tool2 ...]
zhub agent set-mcps <name> <mcp1> [mcp2 ...]
zhub agent set-skills <name> <skill1> [skill2 ...]

# Deploy lifecycle
zhub agent deploy <name> [--force]      # deploy to runtime
zhub agent undeploy <name> [--purge]    # archive or permanently delete
zhub agent start <name>                 # restart stopped agent
zhub agent stop <name>                  # stop running agent
zhub agent status <name>                # check deployment status
```

### Provider (6 commands)
```bash
zhub provider list [--output json|table|yaml]
zhub provider get <id> [--output yaml]
zhub provider create --file <provider.yaml> | --json '<json>'
zhub provider update <id> --file <provider.yaml> | --json '<json>'
zhub provider delete <id>
zhub provider probe <id>                         # test stored provider
zhub provider probe --base-url X --api-key Y --protocol Z  # test custom config
```

### Tool (5 commands)

```bash
zhub tool list [--output json|table|yaml]
zhub tool get <name> [--output yaml]
zhub tool create --file <tool.yaml> | --json '<json>' [--output json|table|yaml]
zhub tool update <name> --file <tool.yaml> | --json '<json>' [--output json|table|yaml]
zhub tool delete <name>
```

### MCP (5 commands)

```bash
zhub mcp list [--output json|table|yaml]
zhub mcp get <name> [--output yaml]
zhub mcp create --file <mcp.yaml> | --json '<json>' [--output json|table|yaml]
zhub mcp update <name> --file <mcp.yaml> | --json '<json>' [--output json|table|yaml]
zhub mcp delete <name>
```

### Skill (6 commands)
```bash
zhub skill list [--type expert|community] [--output json|table|yaml]
zhub skill get <name> [--output json|table|yaml]
zhub skill create --from-dir <dir> [--name N] [--title T] [--title-en T] [--description D] [--description-en D] [--type T] [--output json|table|yaml]
zhub skill update <name> --from-dir <dir> [--title T] [--title-en T] [--description D] [--description-en D] [--type T] [--output json|table|yaml]
zhub skill delete <name>
zhub skill download <name> [--output json|table|yaml]
```

## Output Formats

All list/get commands support `--output`:

| Format | Use case |
|--------|----------|
| `table` (default) | Human reading in terminal |
| `json` | Agent parsing (always wrapped in `{ data, meta }`) |
| `yaml` | Single-object details, copy-paste friendly |

## Agent YAML Format

Agent create and update accept YAML through `--file`. Agent update does **not**
accept `--json`; if its YAML provides `id`, that value must exactly match the
positional `<name>`. Agent create always requires YAML `id`.

The preferred native shape keeps agent configuration under `config`:

```yaml
id: research-agent
enabled: true
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
    你是一个研究员。
  maxTurns: 20
  permissionMode: auto
```

For compatibility, the equivalent flat shape is also accepted. `model` is
normalized to `config.modelId`, while the other configuration fields are moved
under `config`:

```yaml
id: research-agent
enabled: true
isDefault: false
title:
  zh: 研究员
  en: Research Agent
description:
  zh: 负责深度研究与总结
  en: Performs deep research and synthesis
model: claude-sonnet-4-5
systemPrompt: |
  你是一个研究员。
maxTurns: 20
permissionMode: auto
```

Do not define the same normalized field in both places (for example, top-level
`model` together with `config.modelId`, or top-level `title` together with
`config.title`); conflicting definitions are rejected. `title` and
`description` must be language-to-string maps.

On create, YAML `enabled` and `isDefault` values are used unless overridden by
CLI state flags. `--enabled` and `--disabled` are an explicit conflict pair and
must not be supplied together. `--default` and `--no-default` use Clipanion's
native last-argument-wins behavior, so the rightmost occurrence determines the
value. If omitted everywhere, create defaults to enabled and not default.
Update reads state only from YAML and leaves omitted state fields unchanged.

See `skills/zhub-management/references/agent.yaml` for a full example.

## Skill Metadata Flags

Both `skill create` and `skill update` accept four localized metadata flags:

- `--title`: display title
- `--title-en`: English display title
- `--description`: Chinese description
- `--description-en`: English description

`skill create` falls back to the skill name for `--title` and to `SKILL.md`
frontmatter for `--description`; the English fields are optional.

## Use with Claude Code / Cursor

The `zhub-management` SKILL lives in this repository at `skills/zhub-management/`.
Symlink or copy it to your agent host's skills directory:

```bash
# From the repository root
ln -s $(pwd)/skills/zhub-management/ ~/.claude/skills/zhub-management

# Or copy
cp -r skills/zhub-management/ ~/.claude/skills/
```

For Cursor, use `~/.cursor/skills/` instead of `~/.claude/skills/`.

Once linked, the agent will automatically invoke `zhub` when you ask it to
manage agents, skills, or providers.

## Security

- **Tokens** stored in `~/.zhub/config.yaml` with file permission `0600`
- **Provider apiKey** never shown in plaintext — masked to `<hidden>` in all output formats
- **MCP headers** never shown in plaintext — masked to `<hidden>` in `mcp get` output (empty header values remain empty)
- **All commands non-interactive** — designed for automation, no prompts or TUI

## Multiple Profiles

```bash
zhub login --url https://staging.hub... --token cli_yyy --profile staging
zhub --profile staging agent list
```

## Development

```bash
cd cli
bun install
bun test                    # 90 tests
bunx tsc --noEmit           # type check
bun run src/index.ts <cmd>  # run without install
```

Publish: `bun publish` (requires npm login + 2FA)

## License

AGPL-3.0
