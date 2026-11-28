---- MODULE FormatInitialization ----
\* upgradeFormat has two independent SDK callers; Cloud's lease is not assumed.
\* ReadVersion is their advisory read; InitializeEmpty is one writer transaction.
\* CopiedLand models SetBucketRoots reopening an initialized empty Volume with
\* a short child list. Ordinary settle, migration and collection are covered by
\* ListVerifier.tla and Settle.tla; here data means any block, root or liveness key.
\* A failed/discarded writer changes no durable state, represented by stuttering.
EXTENDS Naturals
CONSTANT Split
VARIABLES version, data, certificate, phase, sawEmpty
vars == <<version, data, certificate, phase, sawEmpty>>
Callers == {"A", "B"}
Init == /\ version = FALSE /\ data = FALSE /\ certificate = FALSE
        /\ phase = [p \in Callers |-> "read"]
        /\ sawEmpty = [p \in Callers |-> FALSE]
ReadVersion(p) ==
  /\ phase[p] = "read"
  /\ phase' = [phase EXCEPT ![p] = IF version THEN "done" ELSE "initialize"]
  /\ UNCHANGED <<version, data, certificate, sawEmpty>>
\* The defect reads emptiness in a snapshot discarded before its writer opens.
ReadEmpty(p) ==
  /\ Split /\ phase[p] = "initialize"
  /\ sawEmpty' = [sawEmpty EXCEPT ![p] = ~version /\ ~data]
  /\ phase' = [phase EXCEPT ![p] = "write"]
  /\ UNCHANGED <<version, data, certificate>>
InitializeEmpty(p) ==
  /\ phase[p] = IF Split THEN "write" ELSE "initialize"
  /\ LET empty == IF Split THEN sawEmpty[p] ELSE ~version /\ ~data
     IN /\ version' = (version \/ empty)
        /\ certificate' = (certificate \/ empty)
  /\ phase' = [phase EXCEPT ![p] = "done"]
  /\ UNCHANGED <<data, sawEmpty>>
CopiedLand(p) ==
  /\ phase[p] = "done" /\ version /\ ~data
  /\ data' = TRUE /\ certificate' = FALSE
  /\ UNCHANGED <<version, phase, sawEmpty>>
Done == /\ \A p \in Callers: phase[p] = "done"
        /\ UNCHANGED vars
Next == (\E p \in Callers: ReadVersion(p) \/ ReadEmpty(p) \/ InitializeEmpty(p) \/ CopiedLand(p)) \/ Done
Spec == Init /\ [][Next]_vars
CertificateSound == certificate => ~data
====
