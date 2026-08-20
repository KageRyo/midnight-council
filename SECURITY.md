# Security Policy

Midnight Council is an experimental pre-release project. Security fixes and triage are handled on a best-effort basis, and no hosted service or response-time guarantee is provided by this repository.

## Supported versions

| Version | Support |
| --- | --- |
| `main` | Active development and security fixes when practical |
| Tagged releases | Support stated in the release notes |
| Older commits | Not supported |

## Reporting a vulnerability

Please do not open a public issue, discussion, or pull request for a suspected vulnerability. Use GitHub's private vulnerability reporting form:

[Report a vulnerability](https://github.com/KageRyo/midnight-council/security/advisories/new)

Include the affected commit or release, configuration, reproduction steps, expected impact, and any suggested mitigation. Redact reconnect tokens, credentials, private deployment details, player identifiers, and personal data from reports and logs.

If private vulnerability reporting is unavailable, contact the repository maintainers privately through GitHub before sharing details publicly. There is currently no guaranteed acknowledgement or remediation SLA.

## Current security assumptions

Deployments should review these limitations before exposing a server to untrusted users:

- `ALLOWED_ORIGINS` and `TRUSTED_PROXIES` must match the actual browser and proxy topology.
- A reconnect token is a bearer credential for a player or spectator seat; anyone who obtains it may recover that seat.
- Rate limits are process-local and are not a substitute for an edge firewall or distributed abuse controls.
- Rooms, reconnect credentials, and game state are held in process memory and are not an account or audit system.
- The default chat policy allows messages; deployments that need moderation must inject and operate a stricter policy.

For deployment-specific exposure, report the issue to the operator of that hosted instance as well as using this policy when the repository code is affected.

## Disclosure and fixes

Maintainers will investigate valid reports, coordinate a fix when practical, and agree on a disclosure timeline with the reporter when the issue warrants private coordination. Release notes should describe security fixes without publishing exploit details that would put users at unnecessary risk.
