# Tiny Secrets Manager - Ansible Plugin

The Tiny Secrets Manager (TSM) comes with a built-in Ansible lookup plugin. This allows you to dynamically fetch secrets from your TSM instance directly inside your Ansible playbooks without hardcoding or exporting them to environment variables beforehand.

## 1. Installation

You need to place the TSM lookup plugin into your Ansible project's `lookup_plugins` directory, or any directory configured in your `ansible.cfg` for lookup plugins.

### Option A: Download directly (Recommended / Docker Users)
If you are running the server via Docker and haven't cloned the full source code repository, you can download the plugin directly from GitHub:

```bash
mkdir -p ~/.ansible/plugins/lookup/
curl -sSL -o ~/.ansible/plugins/lookup/tsm.py https://raw.githubusercontent.com/abnabnabn/tiny-secrets-manager/main/plugins/ansible/lookup/tsm.py
```

### Option B: Copy to your project (Source Code Users)
If you have cloned the source code and your Ansible playbook is in `/etc/ansible/playbooks`, create a `lookup_plugins` directory alongside it and copy the python script:

```bash
mkdir -p lookup_plugins
cp plugins/ansible/lookup/tsm.py lookup_plugins/
```

### Option C: Global Ansible Installation (Source Code Users)
If you have cloned the source code and want the plugin available globally for all playbooks, copy it to the default Ansible plugins directory (usually `~/.ansible/plugins/lookup/`). 

You can do this automatically by running:
```bash
make install-ansible-plugin
```

Or manually:
```bash
mkdir -p ~/.ansible/plugins/lookup/
cp plugins/ansible/lookup/tsm.py ~/.ansible/plugins/lookup/
```

## 2. Configuration & Authentication

The Ansible plugin needs to know your server URL and authenticate with a valid Role token. It supports two methods of authentication:

### Method A: Automatic Context Resolution (Recommended)

If you are using the `tsm` CLI, the Ansible plugin will automatically read your `~/.tsm.json` file. It will automatically detect the server URL and resolve the correct machine token based on the directory where you run `ansible-playbook`.

```bash
# Just link your ansible project directory!
cd ~/ansible-project
tsm login https://tsm.yourdomain.com
tsm auth --link <YOUR_MACHINE_TOKEN>

# The plugin will now automatically use this token!
ansible-playbook site.yml
```

### Method B: Environment Variables

If you are running Ansible in a CI/CD pipeline or an environment without the `tsm` CLI installed, you can pass the configuration directly via environment variables:

- `TSM_URL`: The base URL of your Tiny Secrets Manager instance (e.g., `https://tsm.yourdomain.com`).
- `TSM_TOKEN`: A valid Role Token generated from the TSM UI or CLI.

```bash
export TSM_URL="http://localhost:8090"
export TSM_TOKEN="tsm_..."
ansible-playbook site.yml
```

## 3. Usage in Playbooks

Once installed and configured, you can use the `tsm` lookup plugin anywhere in your playbooks using the standard Jinja2 `lookup` syntax.

### Basic Example

```yaml
---
- name: Example playbook using TSM
  hosts: localhost
  tasks:
    - name: Print a secret (Be careful not to log sensitive data!)
      ansible.builtin.debug:
        msg: "The database password is: {{ lookup('tsm', 'app.db.password') }}"
```

### Using as a Variable

You can assign fetched secrets directly to Ansible variables:

```yaml
---
- name: Deploy Database
  hosts: db_servers
  vars:
    db_password: "{{ lookup('tsm', 'prod.db.password') }}"
  tasks:
    - name: Configure database
      ansible.builtin.template:
        src: my.cnf.j2
        dest: /etc/mysql/my.cnf
```

## Troubleshooting

- **Secret not found**: Ensure the secret key exactly matches the key in TSM.
- **Access denied**: Check your `TSM_TOKEN` permissions in the TSM Role Manager. The role associated with the token must have the `GET` permission for the prefix covering your secret.
- **Plugin not found**: Ensure `tsm.py` is in the correct `lookup_plugins` directory and that Ansible is configured to look there. You can verify your plugin paths by running `ansible-config dump | grep LOOKUP`.
