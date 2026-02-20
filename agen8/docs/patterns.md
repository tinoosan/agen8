## Constructor Patterns

Use these patterns for new code and when updating existing components.

### Decision Matrix

- Explicit parameters: simple components with <= 3 required params and no optionals
- Functional options: medium complexity (3-5 params) with optional settings
- Struct config: complex components with > 5 params or many optional settings

### Rules of Thumb

- Validate at constructor entry before any side effects
- Apply defaults in a single place (a `WithDefaults()` or `Default()` helper)
- Return errors from options when validation can fail

### Examples

- Explicit parameters: `NewFS()`, `NewDirResource(baseDir, mount)`
- Functional options: `NewDefaultAgent(opts...)`
- Struct config: `Build(runtime.BuildConfig)`, `session.New(session.Config)`
