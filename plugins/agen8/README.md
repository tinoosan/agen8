# Agen8 Codex Plugin

This plugin packages Agen8 for Codex.

It includes:

- the Agen8 workflow skill
- a local MCP server entry named `agen8`
- setup metadata for the stable local endpoint `http://127.0.0.1:7777/mcp`

## Requirements

Agen8 must be running locally before Codex can use the MCP tools.

Codex must also have the API key available as `AGEN8_MCP_TOKEN`:

```sh
launchctl setenv AGEN8_MCP_TOKEN 'ak_your_agen8_api_key'
```

Restart Codex after setting the token.

## Enterprise Policy

This plugin does not bypass managed Codex MCP policy. Enterprise accounts that
use an MCP allowlist still need to allow Agen8:

```toml
[mcp_servers.agen8]
identity = { url = "http://127.0.0.1:7777/mcp" }
```

Without that allowlist entry, Codex may show the plugin as installed but still
withhold the Agen8 MCP tools from threads.
