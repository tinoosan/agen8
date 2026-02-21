// Package validate provides validation helpers for the agen8.
//
// It offers common validation functions to ensure configurations, inputs, and
// internal states meet expected criteria, helping to fail fast and provide
// clear error messages.
//
// # Error Field Names
//
// Many helpers accept a "name" parameter (for example, `NonEmpty(name, value)`).
// Prefer using lowerCamel identifiers that match the public API/config key when possible
// (e.g. `runId`, `sessionId`, `workdirAbs`, `toolId`) so error messages are consistent
// across layers.
package validate
