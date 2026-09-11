# EXP-LIMIAR-016 — SQLite native macOS runtime

Authority: Experiment
Status: In Progress

## Hypothesis

The `internal/storage/sqlite` implementation that already compiles for Darwin and runs natively on Linux X64/ARM64 with `CGO_ENABLED=0` can execute its existing test suite on a native macOS Travis worker without changing production storage code.

## Why this experiment exists

EXP-LIMIAR-014 established cross-build portability for `darwin/arm64`, but cross-compilation is not runtime evidence. EXP-LIMIAR-015 then established native runtime evidence for Linux ARM64. The remaining baseline gap includes native macOS and Windows execution.

This experiment narrows only the macOS runtime question. It does not declare product-wide macOS support and does not change ADR 019, ADR 021 or ADR 022 authority.

## Harness

A dedicated Travis matrix job uses:

- `os: osx`;
- Go 1.26.2 from the repository CI baseline;
- `CGO_ENABLED=0`;
- an explicit `GOOS=darwin` assertion;
- `go test -v -count=1 ./internal/storage/sqlite`.

The job reports the native `GOARCH` selected by Travis instead of assuming an architecture before the worker is provisioned.

## Evidence required for promotion

Promote this experiment to `Supported` only if a concrete PR HEAD receives a native macOS Travis worker and the SQLite suite completes successfully on that same HEAD.

Provisioning failure, unsupported Travis configuration, quota exhaustion or an unavailable macOS image are CI-infrastructure results and must not be recorded as a SQLite runtime failure.

## Boundaries

A green result supports only the tested `internal/storage/sqlite` package on the actual macOS/architecture/Go combination reported by the runner. It does not establish:

- support for the complete Limiar executable on macOS;
- Windows runtime support;
- other macOS architectures not exercised by the worker;
- Telegram/MTProto runtime behavior on macOS;
- a production support policy.

## Related evidence

- ADR 019 — local SQLite storage baseline;
- EXP-LIMIAR-013 — CGO-disabled storage gate;
- EXP-LIMIAR-014 — SQLite cross-build portability;
- EXP-LIMIAR-015 — native Linux ARM64 SQLite runtime.
