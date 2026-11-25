# Operating Your Own RAMServer Instance

This guide is for developers who want to run a server based on RAMServer-Core using their own hosting account, infrastructure, configuration, and maintenance process.

## Independent deployments

The recommended approach is to fork the RAMServer-Core repository and maintain your deployment from that fork. You are responsible for the code, configuration, infrastructure, access controls, costs, monitoring, backups, updates, and conduct associated with your instance.

A deployment based on this project is not operated by the RAMServer maintainers unless the maintainers explicitly state otherwise. Do not present an independently operated server as an official RAMServer service or imply that the project maintainers endorse or supervise its users.

## Recommended workflow

1. Fork the official repository.
2. Clone your fork and create a deployment branch or release tag for your own changes.
3. Review the configuration, network exposure, persistence path, and secrets handling.
4. Build and test the server locally.
5. Deploy the image or executable to infrastructure that you control.
6. Configure monitoring, backups, updates, and incident response.
7. Publish your own operator contact and service terms to your users.

Keep your fork's upstream remote so you can review and selectively incorporate future upstream changes. Do not assume that merging upstream changes is risk-free; review release notes and migration requirements first.

## Local operation

The repository includes Docker Compose configuration for local development:

```sh
docker compose up --build
```

For native execution:

```sh
cd server
go run ./cmd/ramserver
```

See the root README and current Compose/Docker configuration for actual ports, environment variables, and volume paths. These values may change between releases.

## Production deployment checklist

Before exposing an instance to the public internet, the operator should independently evaluate and configure:

- Encrypted transport (for example, TLS termination at a trusted reverse proxy or load balancer, if compatible with the deployment).
- Network ingress rules and firewall policy.
- Authentication and identity assumptions in the current protocol.
- Rate limits, connection limits, and resource exhaustion protections.
- Database persistence, backup, retention, and recovery.
- Log access, retention, and protection of player information.
- Dependency and container image updates.
- Monitoring, alerting, and incident response.
- A privacy notice and user-facing terms appropriate to the service.

Do not assume that a development configuration is production-hardened. Review the current release's security documentation and implementation before deployment.

## Operator responsibility

The independent operator is responsible for:
- The legality and appropriateness of games and content hosted on their instance.
- User accounts, player conduct, moderation, and dispute handling.
- Compliance with applicable laws and regulations.
- Data collection, privacy notices, retention, and deletion.
- Infrastructure security, availability, and costs.
- Responding to abuse reports and security incidents.

The RAMServer maintainers do not control independent deployments and cannot guarantee their availability, security, behavior, or compliance. Users of an independently operated instance should contact that instance's operator, not assume the upstream project can intervene.

## Attribution and branding

When redistributing or publishing a fork or derivative work:

- Retain existing copyright notices, license notices, and attribution required by the applicable license.
- Identify RAMServer-Core as the upstream project on which the work is based.
- Preserve RAMServer branding assets included in the repository when redistributing them, subject to their applicable terms.
- Do not remove or obscure project attribution in a way that misleads users about the origin of the work.
- Clearly distinguish your product name, operator, website, and support contact from the official project.

Suggested attribution:

> Built using RAMServer-Core, an open-source reusable authoritative multiplayer server core: https://github.com/stevenlagoy/ramserver-core

This suggested attribution does not replace any license-required notice. The project name and logos may also be subject to trademark or other brand-use rules. A license to use source code does not necessarily grant unrestricted rights to use a project's name or logos.

## License and legal notice

The repository should contain an explicit `LICENSE` file before release. Operators must comply with that license and with any separate terms attached to logos, fonts, images, or other assets. This document describes project expectations; it is not a substitute for legal advice or a license grant.
