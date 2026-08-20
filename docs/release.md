# Release and maintenance

Midnight Council is an experimental pre-release project. Releases are deliberate checkpoints for local or controlled deployments; they do not imply a hosted service, API stability, or production support commitment.

## Version and GitHub display

Use Semantic Versioning for release tags:

```text
vX.Y.Z
```

For example, the first experimental release could use the tag `v0.1.0`. In GitHub's release editor, use the same version in the release title with the project name:

```text
Midnight Council v0.1.0
```

The Git tag and the GitHub Release title are separate fields. Keep them aligned so links, container references, changelog entries, and user-facing release pages are easy to match.

## Preparation checklist

Before creating a release:

1. Start from a clean, up-to-date `main` checkout.
2. Confirm the intended version is appropriate for an experimental project; use `0.x` while compatibility is not promised.
3. Run the required checks: `make fmt-check`, `make mod-verify`, `make vet`, `make test`, `make test-race`, `make js-check`, `make vuln`, and the browser E2E suite.
4. Review the deployment, protocol, security, README, roadmap, and [`CHANGELOG.md`](../CHANGELOG.md) updates.
5. Create a tag named `vX.Y.Z` only after the release commit is reviewed and CI is green.

## GitHub Release checklist

- Use the matching tag and title format `Midnight Council vX.Y.Z`.
- Generate release notes from merged pull requests, then edit them for operationally important changes and known limitations.
- Mark the release as a pre-release while the project remains experimental.
- Do not publish a release automatically from a normal push; create it intentionally after the checklist passes.
- Record configuration changes, protocol changes, migration notes, and rollback guidance when relevant.

## Maintenance after release

After publishing a release, watch CI and issue reports for regressions, document any urgent mitigation in the release notes, and decide whether a patch release or a documented workaround is appropriate. Keep dependency, Go, Node, action, and container base-image updates flowing through Dependabot and review them as changes to the supported surface.

Do not delete or move a published tag to correct a mistake. If a release is materially wrong, mark it appropriately, document the reason, and create the next patch or pre-release version.
