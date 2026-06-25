# Homelab Dogfood Rollout

This runbook deploys Agen8 into a homelab Kubernetes cluster from the public
release image, then connects local harnesses to that hosted daemon for dogfood
coordination. Keep cluster hostnames and setup tokens out of git.

## Inputs

Set these locally before deploying:

```sh
export AGEN8_IMAGE="ghcr.io/tinoosan/agen8:v0.0.2"
export AGEN8_HOST="agen8.example.com"
export AGEN8_PUBLIC_URL="https://${AGEN8_HOST}"
export AGEN8_SETUP_TOKEN="$(openssl rand -hex 32)"
```

Confirm `kubectl config current-context` points at the homelab cluster before
running any apply command.

## Deploy

Create the namespace and setup-token secret first:

```sh
kubectl create namespace agen8 --dry-run=client -o yaml | kubectl apply -f -
kubectl -n agen8 create secret generic agen8-secrets \
  --from-literal=AGEN8_SETUP_TOKEN="${AGEN8_SETUP_TOKEN}" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Render the sample manifest with the homelab host and release image:

```sh
tmp_manifest="$(mktemp)"
sed \
  -e "s#ghcr.io/tinoosan/agen8:v0.0.1#${AGEN8_IMAGE}#g" \
  -e "s#https://agen8.example.com#${AGEN8_PUBLIC_URL}#g" \
  -e "s#agen8.example.com#${AGEN8_HOST}#g" \
  deploy/kubernetes/agen8.yaml > "${tmp_manifest}"
kubectl apply -f "${tmp_manifest}"
rm -f "${tmp_manifest}"
```

The sample manifest keeps one replica with a `Recreate` strategy because the
default SQLite storage is single-writer. Do not scale this deployment above one
replica unless storage changes.

## Verify

```sh
kubectl -n agen8 rollout status deployment/agen8
kubectl -n agen8 get pods,svc,ingress,pvc
curl -fsS "${AGEN8_PUBLIC_URL}/healthz"
```

Open the setup URL once:

```sh
printf '%s/setup?token=%s\n' "${AGEN8_PUBLIC_URL}" "${AGEN8_SETUP_TOKEN}"
```

Create the first user through the setup page, then create or copy the MCP token
shown by Agen8. Store it locally, not in git.

## Connect Local Harnesses

For Codex, configure an HTTP MCP server using the hosted endpoint and a local
environment variable:

```json
{
  "mcpServers": {
    "agen8": {
      "type": "http",
      "url": "https://agen8.example.com/mcp",
      "bearer_token_env_var": "AGEN8_MCP_TOKEN"
    }
  }
}
```

Set the token on the workstation that runs Codex:

```sh
export AGEN8_MCP_TOKEN="replace-with-token-from-setup"
```

For hosted deployments keep `AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING=true` in the
pod. Install Codex/Claude hooks on each workstation so hook events originate
from the harness host, not from inside the Kubernetes pod.
