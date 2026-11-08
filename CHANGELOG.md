# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/2.0.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Versions up to 0.11.2 have no entries here.

## [Unreleased]

## [0.14.0] - 2026-11-08

### Added

- `email_create` answers an email named by `in_reply_to`: the draft carries its In-Reply-To and References, and an empty subject or recipient list is taken from it.

### Changed

- `email_create` no longer requires `subject` when `in_reply_to` is set.

## [0.13.0] - 2026-11-08

### Added

- `email_create` takes `identity_id`, the sender identity whose name and address go into From.

### Fixed

- Drafts from `email_create` carry a From header: the first sender identity's, unless `identity_id` names another.

## [0.12.1] - 2026-11-06

### Fixed

- The image runs on arm64: it is published for linux/amd64 and linux/arm64.

## [0.12.0] - 2026-11-06

### Added

- `.deb` packages for amd64 and arm64 in the `sarak.sh` Debian registry and on each release.

### Changed

- **Breaking:** the module path is `sarak.sh/mikluko/jmap-mcp`.
- **Breaking:** the image is `sarak.sh/mikluko/jmap-mcp` and the chart `oci://sarak.sh/mikluko/charts/jmap-mcp`; nothing is published to `ghcr.io` any more.
- The chart's default image repository is `sarak.sh/mikluko/jmap-mcp`.

### Removed

- **Breaking:** the server is no longer published to the MCP Registry as `io.github.mikluko/jmap-mcp`.

[Unreleased]: https://sarak.sh/mikluko/jmap-mcp/compare/v0.14.0...HEAD
[0.14.0]: https://sarak.sh/mikluko/jmap-mcp/compare/v0.13.0...v0.14.0
[0.13.0]: https://sarak.sh/mikluko/jmap-mcp/compare/v0.12.1...v0.13.0
[0.12.1]: https://sarak.sh/mikluko/jmap-mcp/compare/v0.12.0...v0.12.1
[0.12.0]: https://sarak.sh/mikluko/jmap-mcp/compare/v0.11.2...v0.12.0
