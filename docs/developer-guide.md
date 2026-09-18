# Tiny Secrets Manager - Developer Guide

Welcome to the **Tiny Secrets Manager (TSM)** developer guide! This document provides a complete technical walkthrough for contributors and maintainers working on this codebase.

---

## Table of Contents

1. [Architecture & Component Overview](#architecture--component-overview)
2. [Prerequisites & Environment Setup](#prerequisites--environment-setup)
3. [Building & Running Locally](#building--running-locally)
4. [Frontend Development & Prebuild Pipeline](#frontend-development--prebuild-pipeline)
5. [Storage & Cryptographic Architecture](#storage--cryptographic-architecture)
6. [Testing & Quality Assurance](#testing--quality-assurance)
7. [Release Process (Cutting a New Version)](#release-process-cutting-a-new-version)
8. [Ansible Plugin Development](#ansible-plugin-development)
9. [Code Style & Conventions](#code-style--conventions)

---

## Architecture & Component Overview

Tiny Secrets Manager is designed to be an ultra-lightweight, zero-external-runtime secrets engine with built-in disaster recovery, web GUI, and CLI.

### Directory Structure

```text
tiny-secrets-manager/
├── cmd/
│   ├── tsm-server/        # Entrypoint for the server daemon & embedded Web UI
│   ├── tsm-cli/           # Entrypoint for the 'tsm' command-line interface
│   └── prebuild/          # Go tool that bundles React + Tailwind into public/
├── internal/
│   ├── api/               # HTTP router, middleware, and route handlers (auth, roles, secrets, backup)
│   ├── config/            # Server configuration loading, defaults, and env overrides
│   ├── crypto/            # AEAD envelope encryption (XChaCha20-Poly1305), key generation, slots
│   ├── server/            # HTTP server lifecycle, graceful shutdown, backup daemon
│   └── store/             # Pure-Go SQLite persistence layer (WAL mode, schemas, migrations)
├── ui/                    # React frontend source code (main.jsx, App.jsx, styles)
├── public/                # Bundled frontend assets embedded into Go binary via //go:embed
├── plugins/ansible/       # Ansible lookup plugin (tsm.py)
├── scripts/               # Systemd units, global installer scripts
├── docs/                  # Project documentation & OpenAPI specifications
└── Makefile               # Primary developer task runner and cross-compiler
```

### Key Design Tenets

- **Zero-CGO Pure Go:** We use `modernc.org/sqlite` rather than `mattn/go-sqlite3`. Binaries are statically linked and cross-compile cleanly to any architecture without a C toolchain.
- **Node-free Frontend Pipeline:** The React frontend is compiled using Go-based `esbuild` (`github.com/evanw/esbuild`) and a standalone Tailwind CSS CLI binary. Developers do **not** need Node.js or `npm` installed.
- **Single-Binary Distribution:** All frontend HTML/JS/CSS assets are embedded into `tsm-server` using Go's `//go:embed` directive.

---

## Prerequisites & Environment Setup

### Required Tools

- **Go:** 1.24+ (recommended matching `go.mod`)
- **Make:** GNU Make
- **Git:** 2.x+

### Optional Tools (for linting and security scans)

- `golangci-lint`: [Installation instructions](https://golangci-lint.run/welcome/install/)
- `gosec`: `go install github.com/securego/gosec/v2/cmd/gosec@latest`
- `govulncheck`: `go install golang.org/x/vuln/cmd/govulncheck@latest`

### First-Time Workspace Setup

Run the setup target to fetch dependencies, verify linters, run tests, and compile initial binaries:

```bash
make setup
```

---

## Building & Running Locally

The `Makefile` is the primary entry point for all development tasks.

| Command | Description |
|---|---|
| `make build` | Builds both `tiny-secrets-manager` and `tsm` into `bin/<host-platform>/` |
| `make build-server` | Rebuilds the UI assets and compiles the server binary |
| `make build-cli` | Compiles the CLI binary only |
| `make build-all` | Cross-compiles for macOS (ARM64), Linux (AMD64 & ARM64), and Windows (AMD64 & ARM64) |
| `make run` | Builds and launches the server using `config.json` |
| `make run-env` | Builds and launches the server using environment variable configuration |
| `make dev-link` | Symlinks current build binaries into `~/.local/bin` for rapid testing |
| `make dev-unlink` | Removes local symlinks from `~/.local/bin` |
| `make clean` | Removes compiled binaries, generated assets, and temporary test databases |

### Starting the Server in Development

```bash
make run
```

On first run, the server will automatically:
1. Initialize an encrypted SQLite database in WAL mode (`secrets.db`).
2. Generate an admin master key and 3 emergency recovery keys.
3. Print the bootstrap credentials directly to stdout.

---

## Frontend Development & Prebuild Pipeline

The Web UI is located in `ui/` and is written in React 18 with Tailwind CSS.

### How the Asset Pipeline Works

1. When you run `make build-server` or `make build`, Go executes [`cmd/prebuild/main.go`](file:///z:/code/github/tiny-secrets-manager/cmd/prebuild/main.go).
2. `cmd/prebuild`:
   - Automatically downloads the standalone Tailwind CSS CLI binary for your OS into `bin/` (if not already present).
   - Fetches production React/ReactDOM bundles into `public/assets/`.
   - Bundles `ui/main.jsx` and `ui/App.jsx` using the embedded `esbuild` library into `public/assets/bundle.js`.
   - Compiles `ui/style.css` using Tailwind into `public/assets/style.css`.
3. The server binary embeds `public/` using `//go:embed public/*` in `cmd/tsm-server/main.go`.

### Editing the UI

To iterate on the frontend:
1. Modify `ui/App.jsx` or `ui/style.css`.
2. Run `make build-server`.
3. Restart or refresh the server at `http://localhost:8080`.

---

## Storage & Cryptographic Architecture

### Cryptographic Envelope

Located in [`internal/crypto/`](file:///z:/code/github/tiny-secrets-manager/internal/crypto):
- **Cipher:** XChaCha20-Poly1305 (authenticated encryption with 192-bit nonce).
- **DEK (Data Encryption Key):** An ephemeral 256-bit key used to encrypt secrets at rest in SQLite.
- **Key Slots:** The DEK is encrypted into multiple slots:
  - Slot 0: Primary Master Key (derived from password or admin secret).
  - Slots 1–3: Emergency Recovery Keys.
- The DEK never touches disk in plaintext; it is decrypted only in-memory upon server unlock.

### Database Persistence

Located in [`internal/store/`](file:///z:/code/github/tiny-secrets-manager/internal/store):
- Pure Go SQLite runtime (`modernc.org/sqlite`).
- SQLite runs with `PRAGMA journal_mode=WAL` and `PRAGMA foreign_keys=ON`.
- Non-blocking online backups are generated using SQLite's native `VACUUM INTO` API.

---

## Testing & Quality Assurance

Always ensure all checks pass before opening a pull request.

```bash
# Run format, vet, golangci-lint, gosec AST scan, vulncheck, and tests in one go:
make all
```

### Individual Test Commands

- **Unit & Integration Tests (with Race Detection):**
  ```bash
  make test
  ```
  Runs `go test -v -race -coverprofile=coverage.out ./...`.

- **Formatting:**
  ```bash
  make fmt
  ```

- **Linting:**
  ```bash
  make lint
  ```
  Runs `go vet ./...` followed by `golangci-lint run`.

- **AST Security Scanner (gosec):**
  ```bash
  make gosec
  ```

- **Go Vulnerability Scanner:**
  ```bash
  make vulncheck
  ```

---

## Release Process (Cutting a New Version)

Releases are automated via GitHub Actions using semantic version tags (`vX.Y.Z`).

### 1. Versioning Mechanism

The project derives its version at build time from git tags:
```makefile
GIT_TAG := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
VERSION := $(patsubst v%,%,$(GIT_TAG))
LDFLAGS := -s -w -X main.Version=$(VERSION)
```
When tag `v1.0.3` is pushed, `VERSION` becomes `1.0.3` and is embedded into both `tiny-secrets-manager` and `tsm`.

### 2. Pre-Release Checklist

1. Make sure your local branch is clean and synced with `origin/main`:
   ```bash
   git checkout main
   git pull origin main
   ```
2. Run the complete quality suite:
   ```bash
   make all
   ```
3. Verify cross-platform compilation works locally:
   ```bash
   make build-all
   ```

### 3. Publishing the Release

Create and push an annotated git tag:

```bash
# Example for release 1.0.3:
git tag -a v1.0.3 -m "Release v1.0.3"
git push origin v1.0.3
```

*(Alternatively, you can use the GitHub CLI: `gh release create v1.0.3 --generate-notes`)*

### 4. What Happens Automatically in GitHub Actions

Pushing tag `v*.*.*` triggers two automated workflows:

1. **Binary Release Workflow (`.github/workflows/release.yml`):**
   - Compiles all target binaries via `make build-all`.
   - Packages archives for:
     - `darwin-arm64` (.tar.gz)
     - `linux-amd64` (.tar.gz)
     - `linux-arm64` (.tar.gz)
     - `windows-amd64` (.zip)
     - `windows-arm64` (.zip)
   - Creates the GitHub Release with the tag notes and attaches all archive files.

2. **Docker Publish Workflow (`.github/workflows/docker-publish.yml`):**
   - Runs linting and security scans.
   - Builds multi-platform container images (Linux AMD64 and Linux ARM64).
   - Publishes to GitHub Container Registry (GHCR):
     - `ghcr.io/<owner>/tiny-secrets-manager:v1.0.3`
     - `ghcr.io/<owner>/tiny-secrets-manager:1.0.3`
     - `ghcr.io/<owner>/tiny-secrets-manager:latest`

### 5. Post-Release Verification

- Check the **Releases** page on GitHub to verify all 10 assets (`tiny-secrets-manager-*` and `tsm-cli-*`) are attached.
- Verify the universal installer script points to the new version:
  ```bash
  curl -sSL https://raw.githubusercontent.com/<owner>/tiny-secrets-manager/main/public/install.sh | bash -s -- --dry-run
  ```
- Verify the published container image:
  ```bash
  docker pull ghcr.io/<owner>/tiny-secrets-manager:v1.0.3
  ```

---

## Ansible Plugin Development

The repository includes an Ansible lookup plugin in [`plugins/ansible/tsm.py`](file:///z:/code/github/tiny-secrets-manager/plugins/ansible/tsm.py).

To test and install the plugin locally:
```bash
make install-ansible-plugin
```
This symlinks or copies the plugin into `~/.ansible/plugins/lookup/tsm.py`. For usage instructions, refer to [`docs/ansible-plugin.md`](file:///z:/code/github/tiny-secrets-manager/docs/ansible-plugin.md).

---

## Code Style & Conventions

- **Standard Go formatting:** Always run `go fmt` (or `make fmt`).
- **Error Handling:** Explicit error handling without ignoring errors. Avoid silent ignores unless guarded with appropriate security lint comments (`#nosec`).
- **Concurrency & Locking:** When modifying internal store or server state, ensure mutex guards or SQLite WAL transactions are correctly scoped.
- **Commit Messages:** Use clear, descriptive commit messages (e.g. `feat(api): add secret expiration support` or `fix(crypto): handle slot zero rotation`).
