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

## Branches and pull requests

Start new work from `main` and use a descriptive branch name such as `feature/short-name` or `fix/short-name`. Keep pull requests focused and describe the user-visible behavior, design decisions, related issues, and any compatibility or security impact.

Use the pull-request checklist as a final review pass. Do not include credentials, reconnect tokens, private deployment details, generated browser artifacts, or unrelated local files. Report security vulnerabilities through [`SECURITY.md`](SECURITY.md) instead of a public issue or pull request.

## GitHub Flow

This project follows GitHub Flow:

1. Start from an up-to-date `main` branch and create a short-lived feature or fix branch.
2. Make focused commits and keep the branch limited to one coherent change.
3. Push the branch and open a pull request against `main`; use a draft pull request while the work is still in progress.
4. Mark the pull request ready for review only when the description, tests, documentation, and security or operational impact are ready for review.
5. Address review feedback and keep the branch's required CI checks green. A maintainer approval is required before merging.
6. Merge the approved pull request into `main`, then delete the short-lived branch.

The `main` branch should remain buildable and suitable as the starting point for the next change.

## Before Push

Run the relevant checks before every push:

```bash
make fmt
make fmt-check
make mod-verify
make vet
make test
make test-race
make js-check
make vuln
```

The vulnerability check requires network access to download `govulncheck` and its database. Do not push if a required check is failing. When changing the WebSocket or browser flow, also run the Playwright suite described in [`README.md`](README.md).

When changing `internal/webui/static/app.js` and Node.js is available, also run:

```bash
node --check internal/webui/static/app.js
```

Do not add generated browser artifacts from `.playwright-cli/` or `output/`. Do not commit local agent guidance such as `AGENTS.md` unless the repository owner explicitly asks for it.
