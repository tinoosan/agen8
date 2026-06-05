# Frontend Quality Rules

Rules to prevent common React anti-patterns from being reintroduced. Written after an audit of the web frontend.

## Effects

- **Effects are for external systems only**: network subscriptions, timers, imperative browser APIs, third-party widgets, manual DOM integration.
- **Never use `useEffect` to derive state**: if a value can be computed from existing state/props, compute it during render or with `useMemo`.
- **Never use `useEffect` to handle user events**: that logic belongs in event handlers (onClick, onChange, onSubmit, etc.).
- **Never call hooks inside loops, conditions, or `.map()`**: this violates Rules of Hooks and causes unpredictable bugs. If you need per-item data, move the hook into a child component that renders once per item.

## State

- **Don't store derived data in state**: if you can compute it from existing state or props (filtering, sorting, mapping, combining), compute it during render.
- **Use discriminated status over multiple booleans**: prefer `status: 'idle' | 'loading' | 'success' | 'error'` over separate `isLoading`, `hasError`, `hasData` flags that can contradict each other.
- **Don't copy props into state** unless you need an editing buffer that the user can discard.
- **Colocate state with its nearest owner**: don't lift state higher than necessary; don't duplicate state across siblings.
- **URL-driven filters**: if a filter or selection should be shareable, deep-linkable, or honored by external navigation (e.g. notification links), derive it from the URL on every render — do not initialize it into `useState`. Update the filter by navigating with the new query param. `useState(initialValueFromURL)` reads the URL once on mount and then diverges; URL-derived values stay in sync with the address bar forever.

## Shared utilities

- **Before writing a helper, check `lib/`**: formatting, display sanitizers, status labels, file refs, graph helpers, and similar utilities already exist.
- **Project-first language**: new UI copy, test fixtures, and helper names should use project, member, mission, key result, task, decision, graph, file, credential, or HTTP terminology. Do not reintroduce space, plan, chat, operator, escalation, or harness-management language unless the code is intentionally rejecting or documenting a removed concept.
- **Auto-dismiss banners** use `hooks/useAutoDismiss.ts`. Do not copy-paste the `useEffect + setTimeout + setBanner(null)` pattern.

## Accessibility

- **Clickable elements must be `<button>` or `<a>`**: never attach `onClick` to a `<div>` or `<span>`. Buttons get keyboard support (Enter/Space), focus ring, and screen reader semantics for free.
- **Add `aria-expanded`** to expand/collapse toggles.
- **Inputs need labels**: `placeholder` is not a label. Use `<label htmlFor>` or `aria-label`.
- **Icon-only buttons need `aria-label`** or `title` describing their action.

## Component structure

- **Keep page files under ~400 lines**: if a page grows beyond that, extract sub-components into the same directory or a subdirectory.
- **Extract pure helpers outside components**: functions that don't use hooks or component state should be module-level or in `lib/`. This avoids recreation on every render and makes them testable.
- **Don't use inline style objects in hot render paths**: extract them to `const` at module level or use Tailwind classes.

## Dead code

- **Delete unused exports promptly**: they add confusion and maintenance burden. If nothing imports a function, remove it.
- **Don't alias store functions to misleading names**: if a surface needs new state, name it after the retained project-first concept it actually controls.

## Toast defaults

- The store's `addToast` defaults to type `'info'`. Always pass the correct type explicitly: `'error'` for failures, `'success'` for confirmations, `'info'` for neutral messages.

## Performance

- **Don't cargo-cult memoization**: only use `useMemo`/`useCallback` when there is a clear render-path reason (expensive computation, stable reference for memoized children, query key stability).
- **Use stable keys in lists**: prefer unique IDs over array indices for `key` props when items can reorder.
- **Don't create objects/arrays inline in JSX** passed to memoized children, as it defeats memoization.
