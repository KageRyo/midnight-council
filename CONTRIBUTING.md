# Contributing

## Commits

Use Conventional Commits:

```text
<type>[optional scope]: <description>
```

Examples:

```text
feat: scaffold Go realtime room server
docs: document local development workflow
test: cover room start validation
```

Prefer segmented commits. Keep unrelated changes in separate commits so each commit has one clear purpose.

## Before Push

Run unit tests before every push:

```bash
make test
```

Do not push if unit tests are failing.

