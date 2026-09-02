# Contributing

Thanks for your interest in contributing to agent-hub! 🎉

## Quick Start

1. Fork the repo and clone locally
2. Follow [docs/development.md](docs/development.md) to set up the dev environment
3. Create a branch: `git checkout -b feat/your-feature`
4. Make your changes
5. Run tests:
   ```bash
   go test -v ./...
   cd frontend && npm run test:run
   ```
6. Ensure `gofmt -l .` outputs nothing
7. Open a Pull Request

## Conventions

- **Commits**: [Conventional Commits](https://www.conventionalcommits.org/) — `feat:` / `fix:` / `refactor:` / `chore:` / `style:`
- **Field naming**:
  - DB fields: `snake_case`
  - JSON fields: `camelCase` (exception: MCP protocol payloads — tool schemas, arguments, and tool-result JSON use `snake_case` per the MCP ecosystem convention and the pre-existing `knowledge_search` contract, e.g. `dataset_ids`/`top_k`)
  - Go fields: `PascalCase`
- **i18n**: Core entities maintain both Chinese (`description`) and English (`descriptionEn`) fields (exception: Tool — Hub-side title/description is console-display metadata only per issue #88; the runtime-facing description comes from the tool file itself)
- **Error messages**: User-facing errors in Chinese; internal errors in English with stack

## License

By contributing, you agree that your contributions will be licensed under the [agent-hub License](LICENSE). This includes a grant to the producer to adjust the license terms and use contributed code commercially. See condition 2 of the [LICENSE](LICENSE) for details.
