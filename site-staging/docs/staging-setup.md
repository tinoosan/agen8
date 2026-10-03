# Agen8 staging setup

Status: private Agen8 Dev Site registered. Publishing and plugin verification are in progress.

## Isolation

- The legacy repository has a local `staging` branch. Its current `main` checkout and existing edits remain intact.
- The Site source has a separate `staging` branch checked out in `site-staging/`.
- Production remains on the Site source `main` branch in `site/` and keeps its existing Site ID.
- This checkout's hosting manifest identifies the separate Agen8 Dev Site. Never copy the production ID into this manifest.
- This checkout has no `.wrangler/state`, database, legacy archive, or imported data. Its local database and hosted D1 must start empty.
- Use the dev Site's own returned source repository credential and generated plugin ID. Do not push this checkout to the production Site repository or connect the production plugin for staging tests.

## Credential approval

Creating the separate Site returns a new short-lived repository credential and provisions a separate sign-in client. The user requested approval before new credentials, persistent permissions, or account approvals. The user approved creating the dev Site, its sign-in client, and the short-lived publishing credential. Installation and connection require separate approval.

Agen8 Dev uses its own Site ID in `.openai/hosting.json`. Keep credentials in memory and stdin. Publish this checkout only to that Site's source repository and deployment.

## Dev plugin

After staging is published and reports MCP ready, read that Site with its MCP connection details. Use its returned generated plugin ID, URL, and OAuth resource. Give the plugin the Agen8 Dev name. A manifest pointing at production or an unprovisioned URL is not a dev plugin.

Stop before installing, connecting, or granting permissions until the user approves those actions. Then confirm the coding agent can discover and call the dev plugin's actual exposed tools.

## Required end-to-end check

Run these through the agent's exposed Agen8 Dev plugin tools. Raw HTTP checks or the browser alone do not establish plugin usability.

1. Call `project` with `action=list` and confirm the fresh staging workspace is empty.
2. Call `project` with `action=create` and a title beginning `[STAGING TEST] Agent plugin smoke check`, including a timestamp.
3. Read that exact project with `project` using `action=get`. Confirm its title and objective match the write.
4. Register a stable test agent with `agent`, then create a clearly labelled test task in that project with `task`.
5. Read the exact task with `task` using `action=get`, then call `get_context` for the project. Confirm the test record and registered agent are visible.
6. Record which exposed tool names were called and their returned IDs. Keep all records in staging. Do not claim the dev plugin is usable until these calls succeed.

Production's published version and plugin connection must remain unchanged. No legacy import is part of staging setup.
