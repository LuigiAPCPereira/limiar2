# ADR 005 — Serialize Raw Updates to JSON

## Status

Accepted

## Context

The entire purpose of Phase 1 is to discover the **real** shape of the data
Telegram delivers, before any normalization or schema is designed. We therefore
need to capture updates losslessly and in a form that is easy to inspect and
query later, without committing to a domain schema we do not yet understand.

gotd/td's `tg` types (e.g. `tg.UpdatesClass`, `tg.Message`) are plain Go structs
with exported fields, so `encoding/json` captures their full structure.

## Decision

Serialize each raw update to JSON and store the bytes verbatim. `encodeUpdate`
(`telegram/encode.go`) marshals the `tg.UpdatesClass` from gotd's update handler;
`extractMessages` marshals each `*tg.Message` from history responses. The JSON
bytes become `RawMessage.Payload` and are written to `raw_messages.payload`
(TEXT) unchanged. Every record is stamped `schema_version = 1` so future phases
can evolve the capture format while distinguishing payload generations.

The collector's `MessageHandler` preserves the original bytes (`Payload: update`)
and does not mutate them; the `NoopClassifier` passes the message through
unchanged.

## Consequences

- Lossless, human-readable capture; payloads can be queried as valid JSON and
  inspected to design `limiar-processor`.
- `schema_version` provides a forward-compatibility hook for changing the
  capture shape later.
- JSON is larger than a binary encoding (e.g. gotd's TL bin), but storage is
  cheap and inspectability matters more in Phase 1.
- A single update that fails to encode is logged and skipped rather than
  crashing the receive loop.

## Alternatives considered

- **gotd TL binary encoding** — compact but opaque; defeats the goal of
  inspecting real data. Rejected.
- **A normalized domain schema now** — premature; Phase 1 exists precisely to
  learn the shape first. Rejected.
- **Protobuf/MsgPack** — extra dependency and tooling for no Phase 1 benefit.
