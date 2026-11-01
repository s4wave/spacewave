# Formal verification

TLA+ is the default for behavior, protocols, concurrency and lifecycle checks.
Models and bounded configurations live beside the implementation; run
`bun run check:tla`. A Lean proof does not replace a temporal model.

Lean supplements those models with mathematical and security contracts:

- ConfigChain: authorized configuration transitions, suffix composition and induction.
- Crypto and Envelope: grants, operation and checkpoint authorization.
- Order: deterministic ordering and stable prefixes.
- Rounds: weighted quorum intersection and agreement across rounds.
- Recovery: envelope, grant and enrollment authorization.
- Sync/Auth and the small Sync/Sync composition: authenticated data entry.

Run `bun run lean` for all proofs and the standard-axiom audit, and
`bun run test:lean` for the retained real Go conformance corpus. Cryptographic
primitives are explicit assumptions or observations; Lean does not prove the
underlying cipher implementations. Host mutation, catch-up, exchange, writers,
watches and worker joining use temporal models rather than Lean simulations.
The finite models state their own bounds and omissions; they do not establish
an automatic refinement proof from Go.
