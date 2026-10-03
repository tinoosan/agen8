# Shared work graph UI verification

The graph is the main view in Agen8 Dev. The existing Site palette, branding, sidebar surface and graph canvas colours are preserved. The original pure force layout and compact ring/diamond/goal forms are adapted to hosted snapshots. Legacy RPC, execution hooks, ownership labels and worker code are excluded.

The browser only reads projects, snapshots and node history. There are no creation forms, editors, imports, ownership controls or review queues. Search, project selection, type/state filters, node focus, dragging, zoom and relationship navigation are observational.

Layout keys use sorted node IDs/kinds and relationship IDs/endpoints/types. Content and state changes reuse completed positions; dragged positions remain local. The cache holds at most eight topologies. First layouts yield between approximately eight-millisecond chunks; tests compare positions with the original synchronous simulation and cover cancellation. Measured node sizes persist through refresh and macro zoom transitions.

Desktop and 390px mobile checks use labelled local MCP fixtures with standalone work, decisions, dependencies, two goals sharing one work item, stopped work and finished work. Node details show explanation, context, blocker or stop reason, results/checks, evidence and connections. Earlier changes are expandable and paginated. Account isolation, migration, CAS conflicts and atomic history are covered by SQLite tests. Browser writes return 405 and import returns 404.

Reduced motion disables transitions and removes focus animation. Static graph and connection controls retain their position on press. Refresh has contextual busy feedback. The in-app browser was unavailable; rendered checks used an isolated Playwright browser against the local preview.
