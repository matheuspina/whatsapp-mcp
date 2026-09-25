# Contributing

Thanks for wanting to help. Bug reports, ideas and pull requests are all welcome.

By taking part you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Before you start

- **Bugs and ideas:** open an [issue](https://github.com/matheuspina/whatsapp-mcp/issues). Check for an existing one first, and include steps to reproduce, logs, and your environment (OS, Docker version).
- **Security problems:** do **not** open a public issue. See [SECURITY.md](SECURITY.md).
- **Larger changes:** open an issue to discuss the approach before writing the code.

## Licensing of contributions

This project is released under the [PolyForm Noncommercial License 1.0.0](LICENSE), and the maintainer also offers
commercial licenses ([COMMERCIAL.md](COMMERCIAL.md)). To make that possible, by submitting a contribution you confirm that:

1. you wrote it, or have the right to submit it;
2. it is licensed to everyone under the project's [`LICENSE`](LICENSE); and
3. you grant Matheus Pina a perpetual, worldwide, non-exclusive, royalty-free, irrevocable license to use, modify,
   sublicense and relicense your contribution, including under commercial terms.

You keep the copyright in your contribution. If you cannot agree to this, please do not submit code.

## Setting up

Requirements: Docker with Compose 2.24 or later. For local development also Go 1.25+, [uv](https://docs.astral.sh/uv/) with Python 3.11+, and Node.js 20+.

```bash
git clone https://github.com/matheuspina/whatsapp-mcp.git
cd whatsapp-mcp
cp .env.example .env      # then set API_KEY, WEB_UI_USERNAME and WEB_UI_PASSWORD
docker compose up -d --build
```

There is no hot reload: code is copied into the images, so after a change run `docker compose up -d --build <service>`.

### Bridge (Go)

```bash
cd whatsapp-bridge
go test -race ./...
gofmt -l .                # should print nothing for files you changed
```

### MCP server (Python)

```bash
cd whatsapp-mcp-server
uv sync --all-extras
uv run python check.py    # quick syntax check, ruff and mypy
uv run pytest --cov=lib -v
```

### Web panel (Next.js)

```bash
cd whatsapp-web-ui
npm ci
npx tsc --noEmit
npx eslint src
npm run build
```

## Code style

- **Go:** `gofmt`, standard conventions, `fmt.Errorf("context: %w", err)` for wrapping, godoc comments on exported identifiers, table-driven tests.
- **Python:** `ruff` and `mypy`, type hints and docstrings on public functions, raise exceptions instead of returning empty results on error.
- **TypeScript:** strict types, keep the panel free of stored credentials (see [authentication](docs/authentication.md)).
- Use the loggers, not `print` or `fmt.Println` (except for the QR code and startup status).

## Pull requests

1. Fork and create a branch: `git checkout -b feat/my-change`.
2. Keep the change focused on one thing, and add or update tests.
3. Update the documentation when behaviour changes.
4. Make sure the checks above pass.
5. Write [Conventional Commit](https://www.conventionalcommits.org) messages in English (`feat(bridge): ...`, `fix(web-ui): ...`, `docs: ...`).
6. Open a pull request and describe what changed and why.

Never commit `.env`, `store/`, session files, message data or real phone numbers. Use obviously fake data in tests and examples.

## Adding an MCP tool

1. Add the Go endpoint in `whatsapp-bridge/internal/api/handlers.go` and its route in `server.go`.
2. Add the Python function in `whatsapp-mcp-server/whatsapp.py`.
3. Prefer extending an existing action-based tool in `main.py`. Add a new tool only for a distinct task, not for a single endpoint.
4. Assign it to a toolset and give it a title and annotations (read-only, destructive, and so on). Use `Literal[...]` for action parameters.
5. Add or update tests in `whatsapp-mcp-server/tests/test_main_tools.py`.
6. Update the tool tables in the README.

More on the design in [docs/architecture.md](docs/architecture.md).
