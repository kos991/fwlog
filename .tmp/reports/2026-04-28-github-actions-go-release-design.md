# GitHub Actions Design For Go Service Release

## Overview

This repository will keep only the Go service delivery line in GitHub Actions.
Legacy shell-script release artifacts are out of scope for CI/CD automation.

The pipeline will have two responsibilities:

1. Validate the Go service on every push to `main` and on pull requests.
2. Build and publish release artifacts only when a version tag is pushed.

## Goals

- Keep CI focused on the current Go service implementation.
- Produce release-ready artifacts without bundling local logs, indexes, temp data, or build leftovers.
- Support these release targets:
  - `linux/amd64`
  - `linux/arm64`
  - `windows/amd64` debug binary

## Non-Goals

- No GitHub Actions support for the legacy shell-script product line.
- No `.deb` packaging in this phase.
- No auto-deploy to servers in this phase.
- No Windows deployment archive in this phase.

## Release Artifacts

### Linux

For each Linux target:

- standalone binary
- deployment archive `tar.gz`

Proposed names:

- `nat-query-service_linux_amd64`
- `nat-query-service_linux_arm64`
- `nat-query-service_linux_amd64.tar.gz`
- `nat-query-service_linux_arm64.tar.gz`

Each Linux deployment archive will contain:

- service binary
- `nat-query-service.service`
- short release README

### Windows

For Windows:

- debug binary only

Proposed name:

- `nat-query-service_windows_amd64_debug.exe`

## Workflow Layout

### 1. CI Workflow

Path:

- `.github/workflows/ci.yml`

Triggers:

- push to `main`
- pull_request targeting `main`

Responsibilities:

- checkout repository
- setup Go
- download modules
- validate build for the Go service
- verify required packaging files exist

Recommended checks:

- `go mod download`
- `go build -o /tmp/... main.go`
- cross-build checks for `linux/amd64` and `linux/arm64`

This workflow is a quality gate only. It does not publish release artifacts.

### 2. Release Workflow

Path:

- `.github/workflows/release.yml`

Triggers:

- push tags matching `v*`
- manual trigger via `workflow_dispatch`

Responsibilities:

- checkout repository
- setup Go
- derive version from tag
- build matrix artifacts
- package Linux deployment archives
- upload artifacts to the workflow run
- upload assets to GitHub Release when a release context exists

## Build Matrix

The release workflow will use a matrix like:

- `GOOS=linux`, `GOARCH=amd64`, `suffix=""`
- `GOOS=linux`, `GOARCH=arm64`, `suffix=""`
- `GOOS=windows`, `GOARCH=amd64`, `suffix="_debug.exe"`

Rules:

- Linux builds use release flags such as `-ldflags "-s -w"` unless the codebase needs symbols retained.
- Windows debug build keeps symbols unless a later requirement changes that behavior.
- `CGO_ENABLED` must be chosen explicitly based on actual dependency needs. If DuckDB build requirements block cross-compilation, the workflow must fail clearly rather than silently skip targets.

## Packaging Strategy

Linux deployment archives will be assembled in a temporary packaging directory during the workflow and then compressed.

Archive contents:

- `nat-query-service`
- `nat-query-service.service`
- `README_RELEASE.md` or equivalent generated release note

Excluded from package:

- `data/`
- `build/`
- `.tmp/`
- local logs
- test reports
- old release tarballs

## Versioning

Version source:

- Git tag, for example `v1.2.3`

The version string should be injected into build metadata if the code later defines a version variable. If no version variable exists yet, filenames still use the tag version.

## Error Handling

- CI build failure blocks merge readiness.
- Release build failure stops artifact publication.
- Missing required files such as `main.go`, `go.mod`, or `nat-query-service.service` should fail early with explicit messages.
- Unsupported cross-compilation scenarios must fail visibly.

## Verification

Before considering the implementation complete:

- CI workflow runs successfully on a normal push.
- Release workflow produces:
  - Linux AMD64 binary
  - Linux ARM64 binary
  - Linux AMD64 deployment archive
  - Linux ARM64 deployment archive
  - Windows AMD64 debug binary
- Artifact names match the agreed naming convention.
- No local data or temporary files are included in release archives.

## Risks

### Cross-compilation with DuckDB

The Go service imports DuckDB. If the build path requires CGO and platform-specific libraries, cross-platform GitHub Actions builds may need additional setup or may fail for some targets.

Mitigation:

- start with explicit build commands in CI
- verify whether current Go service can cross-compile cleanly
- if not, either add platform-specific setup or narrow the release matrix with a documented reason

### Current repository layout

The repository still contains legacy scripts and packaging materials. Without explicit artifact selection, it is easy to publish the wrong files.

Mitigation:

- package only selected files
- never archive the repository root directly

## Implementation Scope

Implementation should be limited to:

- GitHub Actions workflow files
- minimal release packaging helpers if required
- minimal documentation updates needed to explain the new release path

No unrelated refactoring should be included in this change.
