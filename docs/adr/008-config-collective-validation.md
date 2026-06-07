# ADR 008 — Collective Config Validation, Separate from Load

## Status

Accepted

## Context

Configuration comes from `LIMIAR_`-prefixed environment variables. Operators
benefit from clear, complete feedback: if several values are missing or invalid,
reporting only the first error forces a frustrating fix-one-rerun loop. We also
want a clean separation between "read and default the config" and "decide whether
it is usable," so defaults can be applied without coupling to validation, and
validation can run explicitly before any I/O.

## Decision

Split configuration into two functions in `internal/config`:

- **`Load(v *viper.Viper)`** — binds the `LIMIAR_` env keys, unmarshals into
  `Config`, and calls `ApplyDefaults`. It does **not** validate.
- **`Validate()`** — checks every field and returns **all** problems at once,
  joined with `errors.Join`, rather than stopping at the first.

`ApplyDefaults` fills optional fields (`DBPath` `./limiar.db`, `LogLevel`
`info`, `LogFormat` `json`, `ShutdownTimeout` `15`, `MaxRetries` `10`,
`IOTimeout` `30s`, `DispatcherBufferSize` `256`, `DBWriterBufferSize` `512`).
Required fields (`AppID`, `APIHash`) are never defaulted. `Validate` enforces
ranges (e.g. `ShutdownTimeout` 1–300, `DispatcherBufferSize` 64–4096,
`DBWriterBufferSize` 128–8192) and enumerations (`LogLevel`, `LogFormat`).

`main.go`'s `run()` calls `config.Load` then `cfg.Validate()` before building the
command tree — fail-fast, before any I/O. `Config.String()` masks `APIHash`.

## Consequences

- An operator sees every misconfiguration in one run (Requirement 7.4/7.5).
- Defaulting and validation are independent and individually testable
  (property tests cover "defaults applied," "rejects invalid," "collects all
  errors," and "masking").
- Validation must be called explicitly; forgetting it would skip the checks — so
  the composition root calls it before anything else.
- Secrets never leak: `APIHash` is masked by `String()` and redacted by the
  logger.

## Alternatives considered

- **Validate inside Load** — couples defaulting and validation, and makes it
  awkward to load-then-inspect; rejected for the explicit fail-fast call site.
- **Fail on first invalid field** — poor operator experience; rejected in favor
  of `errors.Join` aggregation.
- **Struct-tag validation library** — an extra dependency outside the closed
  stack; hand-written checks keep the dependency set minimal.
