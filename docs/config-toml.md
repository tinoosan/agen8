# Runtime Config (`config.toml`) and Onboarding

This document explains how Agen8 resolves runtime configuration, how first-run onboarding works, and how to run reliably in server/headless environments.

## Where `config.toml` lives

Default path:

- `${AGEN8_DATA_DIR}/config.toml`
- If `AGEN8_DATA_DIR` is not set, Agen8 uses its normal data-dir default.

Agen8 auto-creates this file on startup if missing. It only stores non-secret settings.

## What goes in `config.toml`

Example:

```toml
[defaults]
model = "z-ai/GLM-5"
# subagent_model = "openai/gpt-5-mini"
# profile = "general"

[env]
# OPENROUTER_BASE_URL = "https://openrouter.ai/api/v1"

[skills]
# conflict = "keep"

[code_exec]
# venv_path = ""
# required_packages = []
```

Notes:

- Do not store API keys in `config.toml`.
- API keys are loaded from environment variables or OS keychain.
- For a full copy-paste template, see [docs/config.toml.example](config.toml.example).

## `code_exec` configuration

`code_exec` uses a daemon-managed Python virtual environment and reconciles packages from `config.toml`.
The daemon watches `${AGEN8_DATA_DIR}/config.toml` and installs missing packages on save.

Fields:

- `code_exec.venv_path` (string, optional; default `<AGEN8_DATA_DIR>/exec/.venv`)
- `code_exec.required_packages` ([]string of pip package names)

Example:

```toml
[code_exec]
required_packages = ["pandas", "requests", "beautifulsoup4"]
```

If an agent run fails due to a missing module, add the package to `required_packages`; the daemon picks it up and reconciles automatically.

## Resolution order

For runtime options (model/profile/workdir/etc):

1. CLI flags
2. Environment variables
3. `config.toml` values (applied as env defaults)
4. Built-in defaults

For API key (`OPENROUTER_API_KEY`):

1. Environment variable
2. OS keychain (`service=agen8`, account `<provider>.api_key`, default provider `openrouter`)
3. If still missing:
4. TTY: interactive onboarding prompt
5. Non-TTY: fail-fast with setup instructions

## How TTY detection works

Agen8 checks both stdin and stdout:

- interactive mode only when `stdin` is a TTY **and** `stdout` is a TTY
- otherwise treated as headless/non-interactive

In code, this is `term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))`.

## First-run onboarding behavior

When interactive (TTY) and API key is missing:

1. Prompt for provider (default `openrouter`)
2. Prompt for default model (default `z-ai/GLM-5`)
3. Prompt for API key (masked input)
4. Save API key to OS keychain
5. Write/update non-secret defaults in `${dataDir}/config.toml`
6. Continue startup

When non-interactive and API key is missing:

- Startup exits with a clear error and instructions.

## Server/headless recommendation

Use env-injected secrets and keep `config.toml` for non-secrets:

1. Inject `OPENROUTER_API_KEY` from your secret manager (systemd env file, Docker secret, Kubernetes secret).
2. Keep model/profile defaults in `${dataDir}/config.toml`.
3. Start Agen8 daemon normally.

Example (systemd EnvironmentFile style):

```sh
OPENROUTER_API_KEY=...
OPENROUTER_MODEL=z-ai/GLM-5
AGEN8_DATA_DIR=/var/lib/agen8
```

## Quick checks

1. Confirm `config.toml` exists in your data dir.
2. Confirm API key source:
   - env var present, or
   - keychain entry exists (`agen8` / `openrouter.api_key`).
3. If running in CI/server, expect non-interactive mode and pre-provision the key.
