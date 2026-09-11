# EXP-LIMIAR-016 — SQLite native macOS runtime

Authority: Experiment
Status: Inconclusive

## Hypothesis

The `internal/storage/sqlite` implementation that already compiles for Darwin and runs natively on Linux X64/ARM64 with `CGO_ENABLED=0` can execute its existing test suite on a native macOS CI worker without changing production storage code.

## Why this experiment exists

EXP-LIMIAR-014 established cross-build portability for `darwin/arm64`, but cross-compilation is not runtime evidence. EXP-LIMIAR-015 then established native runtime evidence for Linux ARM64. The remaining baseline gap includes native macOS and Windows execution.

This experiment narrowed only the macOS runtime question. It did not declare product-wide macOS support and did not change ADR 019, ADR 021 or ADR 022 authority.

## Initial harness

The first PR HEAD attempted to add a dedicated Travis matrix job with:

- `os: osx`;
- Go 1.26.2 from the repository CI baseline;
- `CGO_ENABLED=0`;
- an explicit `GOOS=darwin` assertion;
- `go test -v -count=1 ./internal/storage/sqlite`.

The design intentionally avoided assuming a CPU architecture until a real worker reported it.

## Evidence

The experiment could not reach runtime execution on Travis:

- Travis CI's current macOS environment documentation states that Travis stopped macOS support on March 31, 2025;
- the Travis check suite created for the initial PR HEAD remained queued without producing a check run, so no native macOS worker executed the SQLite suite;
- review correctly noted that the initial harness also did not pin a macOS image, but the provider-level unavailability is prior to that reproducibility concern;
- Travis' multi-CPU documentation describes `arch` alternatives as Linux-only, so adding `arch: arm64` to the macOS job would not be a valid way to manufacture Apple Silicon Evidence on this provider.

The unavailable Travis macOS job was removed rather than leaving a permanently queued/non-executable gate in repository CI.

## Result

`Inconclusive`.

No SQLite runtime failure was observed. No native macOS SQLite success was observed either. The experiment was blocked at the CI-provider boundary before the hypothesis could be exercised.

## What this means

The existing Darwin cross-build Evidence remains valid, but it is still only compilation Evidence. Native macOS runtime remains unproven.

A future experiment may revisit this question using a CI provider or a real macOS machine that can execute the suite natively. Such an experiment must record the actual macOS version, CPU architecture, Go version and effective CGO mode before any `Supported` conclusion.

## Boundaries

This result does not establish:

- failure or success of `internal/storage/sqlite` on macOS;
- support for the complete Limiar executable on macOS;
- Windows runtime support;
- Intel or Apple Silicon runtime behavior;
- Telegram/MTProto runtime behavior on macOS;
- a production support policy.

## Related evidence

- ADR 019 — local SQLite storage baseline;
- EXP-LIMIAR-013 — CGO-disabled storage gate;
- EXP-LIMIAR-014 — SQLite cross-build portability;
- EXP-LIMIAR-015 — native Linux ARM64 SQLite runtime.
