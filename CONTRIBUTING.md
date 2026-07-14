# Contributing

## Commits

Use [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/):

```text
<type>[optional scope]: <description>
```

Examples:

```text
feat: scaffold Go realtime room server
feat(web): add playable browser client
docs: document local development workflow
test: cover room start validation
```

Use a lowercase type such as `feat`, `fix`, `docs`, `test`, `refactor`, `build`, or `chore`. Add a short scope when it makes the affected area clearer. Use `!` and a `BREAKING CHANGE:` footer when a change is not backward compatible.

Prefer segmented commits. Keep unrelated changes in separate commits so each commit has one clear purpose. Write descriptions in the imperative mood and do not end them with a period.

## Before Push

Run unit tests before every push:

```bash
make fmt
make test
```

Do not push if unit tests are failing.

When changing `internal/webui/static/app.js` and Node.js is available, also run:

```bash
node --check internal/webui/static/app.js
```

Do not add generated browser artifacts from `.playwright-cli/` or `output/`. Do not commit local agent guidance such as `AGENTS.md` unless the repository owner explicitly asks for it.
