# Self-Hosting Agen8

This guide covers Docker and Kubernetes deployments where Agen8 runs somewhere
other than the developer machine.

The important rule is:

> The Agen8 daemon can be hosted remotely, but harness hooks are installed on the
> local machine running Codex or Claude Code.

The hosted daemon receives MCP traffic at `/mcp` and attention hook events at
`/hooks/attention`. It cannot safely install `.claude/settings.local.json` or
`~/.codex/hooks.json` for remote users, so hosted deployments should disable
local hook auto-provisioning.

## Runtime Model

A hosted deployment has two sides:

- Server side: the Agen8 daemon, database files under `/data`, web UI, `/mcp`,
  `/setup`, `/healthz`, `/events`, and `/hooks/attention`.
- Client side: Codex or Claude Code, local Agen8 skill installation, local MCP
  config, and local hook config.

Set these environment variables on the daemon:

```sh
AGEN8_DATA_DIR=/data
AGEN8_PUBLIC_URL=https://agen8.example.com
AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING=true
```

`AGEN8_PUBLIC_URL` must be the URL users can reach from their own machines. Use
HTTPS for any non-local deployment because MCP and hook requests carry bearer
tokens.

Optional:

```sh
AGEN8_SETUP_TOKEN=<long-random-token>
```

If omitted, the daemon generates a one-time setup token and prints the setup URL
to logs. In Kubernetes, setting it explicitly through a Secret is usually easier
than scraping pod logs.

## Docker

Build the image:

```sh
docker build -t agen8:local .
```

Run locally with persistent SQLite data:

```sh
docker run -d \
  --name agen8 \
  -p 7777:7777 \
  -v agen8-data:/data \
  -e AGEN8_PUBLIC_URL=http://127.0.0.1:7777 \
  -e AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING=true \
  agen8:local
```

Run behind a reverse proxy:

```sh
docker run -d \
  --name agen8 \
  -p 7777:7777 \
  -v agen8-data:/data \
  -e AGEN8_PUBLIC_URL=https://agen8.example.com \
  -e AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING=true \
  agen8:local
```

Compose uses the same model:

```sh
AGEN8_PUBLIC_URL=https://agen8.example.com docker compose up --build -d
```

Published release images are pushed to GitHub Container Registry when a `v*`
release tag is created:

```sh
docker pull ghcr.io/tinoosan/agen8:<release-tag>
```

Check health:

```sh
curl -fsS http://127.0.0.1:7777/healthz
docker inspect --format '{{json .State.Health.Status}}' agen8
```

## Kubernetes

The sample manifest is at `deploy/kubernetes/agen8.yaml`.

Before applying it:

1. Replace `replace-with-release-tag` in the image with the immutable release
   tag you intend to deploy.
2. Replace every `agen8.example.com` value with your real hostname.
3. Adjust the ingress class and cert-manager issuer for your cluster.
4. Confirm your storage class supports a `ReadWriteOnce` PVC.

Create the referenced Secret separately so applying the reusable manifest can
never overwrite it:

```sh
export AGEN8_SETUP_TOKEN="$(openssl rand -hex 32)"
kubectl create namespace agen8 --dry-run=client -o yaml | kubectl apply -f -
kubectl -n agen8 create secret generic agen8-secrets \
  --from-literal=AGEN8_SETUP_TOKEN="${AGEN8_SETUP_TOKEN}" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Apply:

```sh
kubectl apply -f deploy/kubernetes/agen8.yaml
```

Watch rollout:

```sh
kubectl -n agen8 rollout status deployment/agen8
kubectl -n agen8 get pods,svc,ingress,pvc
```

Open setup:

```text
https://agen8.example.com/setup?token=<AGEN8_SETUP_TOKEN>
```

The manifest intentionally uses:

- `replicas: 1`
- `strategy.type: Recreate`
- one PVC mounted at `/data`
- storage readiness on `/readyz` and process liveness on `/healthz`
- `AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING=true`

Do not scale the SQLite deployment above one replica. Agen8 intentionally ships
as a single-writer SQLite service.

## Connect A Harness

After first setup, Agen8 returns an API key beginning with `ak_`. Use that key
from the local machine that runs the harness.

Codex MCP config:

```toml
[mcp_servers.agen8]
url = "https://agen8.example.com/mcp"
bearer_token_env_var = "AGEN8_MCP_TOKEN"
```

Then set the token before launching Codex:

```sh
export AGEN8_MCP_TOKEN='ak_...'
```

Claude Code one-command setup:

```sh
cd /path/to/local/project
agen8 client setup --harness claude \
  --url https://agen8.example.com \
  --token ak_...
```

This installs the Claude workflow skills, project attention hooks, and a
local-scope MCP connection together. The Projects page generates the command
for new and existing projects. Display-name changes do not require reconnecting
Claude. Re-run the setup command from the local project directory to repair the
client integration, or pass `--project-dir` explicitly.

When a project folder moves or is renamed, use **Project actions > Change
project folder**. The operation keeps the existing project ID and validates the
new directory on the project's configured location. Hosted setup commands use
project-bound `wlt_` credentials, so generate a fresh command with **Configure
Claude MCP** after relocating the project.

Install the Agen8 workflow skill for Codex locally:

```sh
agen8 skill install --harness codex
```

## Install Attention Hooks

Hooks tell Agen8 when a harness session is waiting, asking, blocked on
approval, or active again. They are local harness configuration, not server
configuration.

Codex:

```sh
agen8 hooks install \
  --harness codex \
  --url https://agen8.example.com \
  --token ak_...
```

Claude Code repair command when only hooks need replacing:

```sh
agen8 hooks install \
  --harness claude \
  --url https://agen8.example.com \
  --token ak_... \
  --project-dir /path/to/local/project
```

These commands write:

- Codex: `~/.codex/hooks.json`
- Claude Code: `/path/to/local/project/.claude/settings.local.json`

Do not run these commands inside the Kubernetes pod unless the harness also runs
inside that pod. For normal self-hosting, each user runs them on their own
machine.

## Security Checklist

- Serve hosted Agen8 over HTTPS.
- Keep API keys out of git, shell history, and shared logs.
- Use `AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING=true` for remote deployments.
- Rotate API keys when a user leaves or a machine is lost.
- Restrict ingress to the hostnames you expect.
- Back up the `/data` volume before upgrades.

## Upgrade Notes

Back up the volume before replacing the binary. Agen8 transactionally migrates
the supported schema-5 baseline to schema 6; older incompatible schemas fail
startup without replacing the database.

For Docker, replace the container while keeping the named volume:

```sh
docker pull <image>
docker rm -f agen8
docker run -d --name agen8 -p 7777:7777 -v agen8-data:/data <image>
```

For Kubernetes, update the image and let the `Recreate` strategy restart the
single pod:

```sh
kubectl -n agen8 set image deployment/agen8 agen8=<image>
kubectl -n agen8 rollout status deployment/agen8
```

Always confirm:

```sh
curl -fsS https://agen8.example.com/healthz
curl -fsS https://agen8.example.com/readyz
```
