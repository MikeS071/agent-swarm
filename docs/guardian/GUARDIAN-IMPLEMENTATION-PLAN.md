# Guardian Implementation Plan

This doc was reconstructed after rollback cleanup.
Use `GUARDIAN-BUILD-SPEC.md` + `GUARDIAN-TICKETS.md` as source of truth.

Execution phases:
1. G1 schema + advisory engine
2. G2 transition enforcement
3. G3 spawn/done enforcement
4. G4 evidence + approvals + report
5. G5 config/init/migrate + defaults

Delivery rule: no ticket marked done until required verify command passes.
