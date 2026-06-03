/// <reference lib="webworker" />

/**
 * Strategy-map layout worker — currently inert.
 *
 * An earlier iteration of `useStrategyGraph` offloaded the force simulation
 * to this worker via `new Worker(new URL(..., import.meta.url), { type: 'module' })`.
 * That path produced broken layouts in certain cases, so the hook now runs
 * `assignClusterMeta` + `runForceLayout` synchronously on the main thread.
 *
 * The file is kept as a stub (rather than deleted) because Vite's dev-mode
 * bundle cache can hold stale references to module URLs across HMR cycles,
 * and a missing module at an expected URL manifests as a blank page. When a
 * future refactor re-enables worker-based layout, replace this stub with the
 * real entry point.
 */
export {}
