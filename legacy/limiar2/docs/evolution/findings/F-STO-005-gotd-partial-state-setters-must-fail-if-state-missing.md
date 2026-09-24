# F-STO-005 — gotd partial state setters must fail if user state is missing

Authority: Non-authoritative Finding
Status: Open

## Discovery

The `github.com/gotd/td/telegram/updates.StateStorage` contract explicitly requires the partial common-state setters `SetPts`, `SetQts`, `SetDate`, `SetSeq`, and `SetDateSeq` to return an error when the user's internal state does not exist.

The physical harness used by EXP-LIMIAR-009/010 implemented those methods with plain SQL `UPDATE` statements and returned only the execution error. In SQLite, an `UPDATE` matching zero rows can succeed, so the harness could report success while persisting no state transition.

## Impact

This does not invalidate the supported ordering scenarios in EXP-LIMIAR-009/010 because those scenarios seed `source_sync_state` before the manager runs. It does mean that the harness did not yet model the complete absence semantics required by gotd, and therefore it is premature to freeze the production `SourceSyncState` adapter/schema from those experiments alone.

The important distinction is:

- `SetState` may create or replace the complete user state;
- partial setters must mutate an existing state only;
- absence must fail rather than fabricate an incomplete row or silently succeed.

## Executable follow-up

`internal/telegram/gotd_state_storage_absence_exp_test.go` exercises every partial common-state setter against a database with no row for the target user and requires an error while verifying that no row is fabricated.

The experimental physical adapter now checks `RowsAffected` and treats zero affected rows as missing state.

## Architectural consequence

A future physical `SourceSyncState` decision must preserve these semantics. In particular, a convenience UPSERT for partial setters would violate the upstream contract by inventing values for fields that were never established by a complete `SetState`.

This Finding should be considered together with ADR 016, ADR 017, EXP-LIMIAR-010, and the channel-state experiment merged in PR #181 before proposing the final physical schema/adapter contract.
