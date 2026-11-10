# Review

What a review of a pull request here checks beyond CI.

- The change does what its bead or issue asks, and nothing else.
- A new tool follows the tool pattern in `AGENTS.md`: an input struct with `json` and `jsonschema` tags, a package-level `mcp.Tool`, and a handler method on `Server` registered in `registerTools()`.
- Tools that send email or change Sieve scripts stay behind `-enable-send` and `-enable-sieve`, which default to off.
- New behaviour has a test.
- A notable change has an entry under `## [Unreleased]` in `CHANGELOG.md`.
- A new version heading in `CHANGELOG.md` is rejected unless the bead or issue asks for a release: once it reaches `main`, `.forgejo/workflows/release.yaml` tags it and publishes the image, the chart and the packages.
