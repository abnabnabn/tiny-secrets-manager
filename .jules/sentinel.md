# Sentinel's Journal

## 2026-07-19 - Remote Backup SCP Argument/Option Injection
**Vulnerability:** The application executes `scp` under the hood to perform remote database backups using `exec.CommandContext`. While this doesn't run in a shell, `scp` itself accepts options/flags (such as `-oProxyCommand`) that can lead to remote command execution if the target settings or destination paths start with a dash (`-`).
**Learning:** Even when avoiding shell execution, passing unsanitized user-controlled arguments to CLI utilities (like `ssh` or `scp`) remains vulnerable to parameter/option injection. The utility interprets leading-dash arguments as configuration flags instead of files/destinations.
**Prevention:** Always validate and trim user inputs before passing them as command arguments. Reject any values starting with a dash (`-`), and explicitly use the end-of-options delimiter (`--`) supported by the CLI utility to prevent subsequent arguments from being parsed as options.

## 2026-07-20 - Schema/Input Boundary Validation for System Settings
**Vulnerability:** The settings storage of the application was vulnerable to mass-assignment/arbitrary key creation via `handlePutSettings`. Furthermore, malformed values like negative integers or dash-prefixed paths were only checked down the line in background daemons/backup execution instead of at the REST input boundary.
**Learning:** Input validation should always be applied as early as possible, ideally at the first entrypoint / HTTP API boundary. Trusting later daemon components to check values can cause unexpected application states, database pollution, or deferred errors.
**Prevention:** Maintain a strict allowlist of recognized settings keys, and perform type/boundary validation on each value (e.g. integer range check, boolean parsing, and prefix verification) before writing to the persistent database.
