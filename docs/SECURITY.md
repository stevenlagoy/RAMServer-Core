# Security

## Security model

RAMServer-Core is designed around server-authoritative game state. The server binds actions to the identity associated with a connection and validates actions against the current match state. Clients must not be trusted to supply authoritative identity, turn state, or game state.

This architecture reduces certain forms of client-side cheating, but it does not make every deployment or game implementation secure by default.

## Known design considerations

The design specification describes a length-prefixed Protocol Buffers wire format over TCP, frame-size limits, connection controls, and server-side validation. The security properties of a deployment depend on the exact release, configuration, network path, and rulesets enabled.

Do not assume that plain TCP provides encryption. Before public deployment, operators should provide transport encryption using a supported deployment architecture and review authentication, rate limiting, denial-of-service exposure, persistence, logging, and secret management.

## Reporting a vulnerability

Do not publish exploit details in a public issue before maintainers have had a reasonable opportunity to investigate.

Until a private security contact is published, use the repository's current GitHub security advisory reporting feature if enabled. If it is unavailable, contact a repository maintainer through a verified project-controlled channel. Do not include credentials or personal data in a report.

A useful report includes:
- Affected release, commit, and deployment configuration.
- Impact and realistic attack prerequisites.
- Reproduction steps or a minimal proof of concept.
- Relevant logs with secrets and personal data removed.
- Suggested mitigation, if known.

## Independent deployments

Operators of forks and independent instances are responsible for their own security response, updates, user communications, and infrastructure. Reporting a vulnerability to the upstream project does not automatically notify or remediate every fork or deployment.
