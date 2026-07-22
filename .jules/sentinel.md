# Sentinel's Journal

## 2026-07-19 - Remote Backup SCP Argument/Option Injection
**Vulnerability:** The application executes `scp` under the hood to perform remote database backups using `exec.CommandContext`. While this doesn't run in a shell, `scp` itself accepts options/flags (such as `-oProxyCommand`) that can lead to remote command execution if the target settings or destination paths start with a dash (`-`).
**Learning:** Even when avoiding shell execution, passing unsanitized user-controlled arguments to CLI utilities (like `ssh` or `scp`) remains vulnerable to parameter/option injection. The utility interprets leading-dash arguments as configuration flags instead of files/destinations.
**Prevention:** Always validate and trim user inputs before passing them as command arguments. Reject any values starting with a dash (`-`), and explicitly use the end-of-options delimiter (`--`) supported by the CLI utility to prevent subsequent arguments from being parsed as options.

## 2026-07-22 - Strict API Boundary Input Validation for System Settings
**Vulnerability:** The system settings PUT endpoint (`/v1/system/settings`) accepted arbitrary key-value pairs without input validation or key whitelisting. This allowed potential configuration pollution, unexpected system behaviors (parameter injection), and evasion of input constraints (such as setting an interval or retention limit to invalid values, or circumventing dash-prefixed target validation).
**Learning:** System configurations that alter security policies, paths, or execution parameters must always be strictly validated and whitelisted at the API input boundary, ensuring no unexpected or malformed options can be persisted.
**Prevention:** Whitelist allowed configuration keys and enforce strict type/range constraints on values (e.g., integer range checks, boolean string limits, format checks for paths/targets) before applying or persisting any settings updates.
