# Sentinel's Journal

## 2026-07-19 - Remote Backup SCP Argument/Option Injection
**Vulnerability:** The application executes `scp` under the hood to perform remote database backups using `exec.CommandContext`. While this doesn't run in a shell, `scp` itself accepts options/flags (such as `-oProxyCommand`) that can lead to remote command execution if the target settings or destination paths start with a dash (`-`).
**Learning:** Even when avoiding shell execution, passing unsanitized user-controlled arguments to CLI utilities (like `ssh` or `scp`) remains vulnerable to parameter/option injection. The utility interprets leading-dash arguments as configuration flags instead of files/destinations.
**Prevention:** Always validate and trim user inputs before passing them as command arguments. Reject any values starting with a dash (`-`), and explicitly use the end-of-options delimiter (`--`) supported by the CLI utility to prevent subsequent arguments from being parsed as options.

## 2026-07-20 - Unvalidated System Settings Injection and Configuration Bloat
**Vulnerability:** The `/v1/system/settings` PUT endpoint allowed administrative clients to write arbitrary key-value configuration settings into the database without restriction or range checking. This allowed malicious or malformed settings (e.g., negative intervals, or values with leading dashes) to bypass client-side checks and disrupt the background backup routine or retention policy.
**Learning:** Trusting admin clients to send well-formed, valid system settings at the API boundary introduces potential security risks (including parameter injection on other CLI tools or denial of service through incorrect backup schedules). Boundary enforcement is essential even for authenticated admin endpoints.
**Prevention:** Always strictly validate and whitelist configuration keys at the API entry point. Validate value types (such as integers >= 1 or boolean strings) and sanitize inputs (like rejecting leading dashes on path configurations) before applying configuration changes.
