# Tiny Secrets Manager

[![Build and Publish Docker Image](https://github.com/abnabnabn/secrets/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/abnabnabn/secrets/actions/workflows/docker-publish.yml)

A lightweight, high-performance, XChaCha20-Poly1305 encrypted secrets manager backed by a pure Go SQLite implementation.
 Designed for localized machine-to-machine (M2M) deployments and administrative ease, providing a secure, hardened enclosure for sensitive configuration.

## Key Features

* **Secure by Design:** Uses XChaCha20-Poly1305 envelope encryption. Secrets are encrypted with an ephemeral 256-bit Data Encryption Key (DEK), which is itself wrapped in multiple "slots" using a primary Master Key and three emergency Recovery Keys.
* **Pure Go SQLite:** Built with `modernc.org/sqlite`, ensuring zero-CGO portability and a simplified supply chain. Operates in WAL mode for high concurrency.
* **Advanced Access Control:** Granular Policy-Based Access Control (PBAC). Tokens can be restricted to specific key prefixes (using dot-notation like `app.dev.db`) and specific operations (`GET`, `LIST`, `PUT`, `DELETE`).
* **Interactive Admin GUI:** A built-in React/Tailwind management interface allows admins to:
    * Manage secrets with full CRUD support.
    * Provision, edit, and clone machine tokens.
    * **Audit Mode:** "View As" any token to simulate and verify its exact permission boundary.
* **Automated Disaster Recovery:** 
    * Every change triggers an automated, consistent database backup using `VACUUM INTO`.
    * Supports both local filesystem targets and remote off-site backups via `scp`.
* **Hardened Deployment:** Optimized for containerization using minimal **Chainguard** base images for maximum security and minimal attack surface.

## Getting Started

### 1. Installation & Setup
Follow these steps in order to prepare and start the Tiny Secrets Manager.

```bash
# 1. Initialize the project (tidies modules and builds all binaries)
make setup

# 2. Start the server
# Option A: Zero-Config (Random credentials will be generated and printed)
make run

# Option B: Custom Admin (Set your own credentials on first boot)
TSM_ADMIN_USER=admin TSM_ADMIN_PASS=mypassword make run
```

### 2. Self-Bootstrapping
The `tiny-secrets-manager` binary is designed to be self-sufficient. If no `config.json` is found on the first run, it will:
1.  **Generate Infrastructure:** Creates a `config.json` with a random 256-bit Master Key.
2.  **Initialize Database:** Creates an encrypted `tsm.db` enclosure.
3.  **Seed Admin:** Creates the initial administrator account.
4.  **Display Credentials:** **The initial username, password, and API token will be printed to the console exactly once.**

### 3. Environment Variables
You can customize the server's behavior by passing environment variables. These can be used with `make run` or when executing the binary directly.

| Variable | Usage | Default |
|----------|-------|---------|
| `TSM_ADMIN_USER` | (Seed Only) Custom username for initial admin. | `admin` |
| `TSM_ADMIN_PASS` | (Seed Only) Custom password for initial admin. | *Random* |
| `TSM_ADMIN_TOKEN`| (Seed Only) Custom API token for initial admin. | *Random* |
| `TSM_MASTER_KEY` | 32-byte Base64 encryption key. | *Auto-generated* |
| `TSM_LISTEN` | Bind address and port. | `0.0.0.0:8090` |
| `TSM_DB_PATH` | Path to the SQLite database file. | `tsm.db` |

**Example: Starting with a custom port and admin password**
```bash
TSM_LISTEN="127.0.0.1:9000" TSM_ADMIN_PASS="secure-password" ./bin/tiny-secrets-manager
```

### 4. Running with Docker (Recommended)
The project includes a hardened `Dockerfile` based on Chainguard images.

```bash
# Start with Docker Compose
docker compose up -d
```
*Note: You can edit the `environment` section in `docker-compose.yaml` to set your initial credentials.*

## API & Integration

### Standalone CLI
The project includes a robust, statically linked Go CLI for managing and injecting secrets.

**1. Setup:**
```bash
# Build the Go CLI
make build-cli
```

**2. Configuration:**
Tiny Secrets Manager uses a **Context-Based Authentication** system. Instead of global environment variables, you link specific directories to specific machine tokens. This information is stored in a protected central file (`~/.tsm.json`).

```bash
# 1. Set your server URL
./bin/tsm login http://localhost:8090

# 2. Link your current project directory to a specific token
cd ~/projects/my-app
./bin/tsm auth --link your-machine-token-here

# 3. Audit active contexts and validate tokens
./bin/tsm auth --status

# 4. Clean up stale or redundant directory mappings
./bin/tsm auth --tidy
```

Once linked, the CLI will automatically use the correct token whenever you are inside that directory or its subdirectories.

**3. Usage Examples:**
```bash
# No token needed - automatically resolved from context
./bin/tsm ls
./bin/tsm get app.db.password
```

**4. Context Management Details:**
*   **`--status`**: Performs a read-only audit of all mapped directories. It masks tokens (e.g., `tsm_tk_abcd...1234`) and pings the server to verify if each token is still valid or has been revoked.
*   **`--tidy`**: Performs local housekeeping. It removes entries for directories that no longer exist on your machine and prunes redundant child-directory mappings if a parent is already linked to the same token.

**4. Running Applications with Secrets:**
The `run` command allows you to execute programs with environment variables injected directly from the secrets manager.

*   **Explicit Mapping:** Create a `tsm.env` file in your project root:
    ```text
    DATABASE_URL=app.prod.db_url
    API_KEY=app.prod.api_key
    ```
*   **Automatic Resolution:** The CLI resolves the token based on your current directory context.
*   **Explicit Token (Override):** You can manually provide a token for a single run using the `--token` flag:
    ```bash
    ./bin/tsm run --token xxxxx -- ./my-app
    ```

### Ansible Integration
To securely fetch secrets dynamically within Ansible playbooks, use the provided lookup plugin in `plugins/ansible/lookup/tsm.py`.

**Example Playbook Usage:**
```yaml
- name: Start Database Container
  community.docker.docker_container:
    name: my_db
    image: postgres:latest
    env:
      POSTGRES_PASSWORD: "{{ lookup('tsm', 'app.db.password') }}"
```

## Development

*   **`make setup`**: Full initialization (tidy + build). Recommended for first-time use.
*   **`make build`**: Compiles stripped, optimized binaries for the server and CLI.
*   **`make run`**: Builds the server and starts it.
*   **`make test`**: Runs the Go test suite with the race detector enabled. (Executed automatically on push via GitHub Actions).
*   **`make clean`**: Removes binaries, local database files, and the `config.json`.
*   **`make tidy`**: Cleans up and synchronizes Go module dependencies.
*   **`make dev-link`**: Creates symlinks in `~/.local/bin` to your local project binaries. Allows you to run `tsm` from any directory while developing. No root required.
*   **`make dev-unlink`**: Removes the development symlinks.
