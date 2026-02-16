# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- MIT LICENSE file
- `--version` flag to CLI (injected via ldflags at build time)
- GoReleaser configuration for cross-platform releases
- GitHub Actions release workflow (triggered on `v*` tags)
- GitHub Actions CI workflow for pull requests
- `install.sh` for curl-pipe-sh installation
- Installation section in README

### Changed
- Makefile now injects version via ldflags
- Updated requirements: macOS listed as supported, Go only needed for source builds

## [0.0.0] - 2026-02-16

### Added
- Initial implementation of caddy-atc local development gateway
- CLI commands: `up`, `down`, `adopt`, `unadopt`, `status`, `routes`, `trust`, `logs`
- Docker event watcher for automatic route management
- Caddyfile generation with hostname-based reverse proxy routing
- HTTPS with auto-generated certificates via Caddy's internal CA
- HTTP service detection from compose files and Dockerfiles
- `start` and `stop` commands with YAML port stripping
- Compose file detection and stripped file generation
- macOS compatibility (POSIX `ps` instead of `/proc`, macOS trust instructions)
- Comprehensive unit tests

### Fixed
- Duplicate hostname grouping into single Caddyfile site blocks
- Auto-start gateway container when stopped during route reload
- Zombie container restart on WSL2 sleep/hibernate
- Actionable hints in "no HTTP port detected" log messages

[Unreleased]: https://github.com/g-brodiei/caddy-atc/compare/v0.0.0...HEAD
[0.0.0]: https://github.com/g-brodiei/caddy-atc/commits/v0.0.0
