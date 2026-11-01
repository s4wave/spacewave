---- MODULE Publication ----
\* Composition of flushJob.run/mergeJob.run -> publishManifest -> writeSuper
\* -> releaseStep. Two independent publishers replace successive manifests.
\* Each publication first has durable output runs (Durability.tla), appends
\* its record under writer.lock (refreshes first), writes its manifest, syncs
\* it, then serializes alternating superblocks with lockSuper. The release
\* watermark advances only after the superblock sync succeeds. A snapshot
\* pins the replaced manifest until Discard. Snapshot acquisition and process
\* slots are abstracted by Pins.tla; this module checks their release boundary.
\* Manifest versions stand for the record seq that installed them: source
\* releaseStep uses pin.seq, not the manifest-version counter. Value chunks
\* use a separate logged watermark, covered in Durability.tla and Free.tla.
\* Errors before a successful sync leave the old durable watermark; stutter
\* represents a retryable failed write/sync. A crash can choose either whole
\* superblock written since the previous sync. Torn blocks fail the CRC and
\* fall back to that previous durable block, included in candidates.
\* Excludes byte encoding, CRC collisions, link records and tombstone merge
\* ordering; real device/crash and merge checks cover these separately.
EXTENDS Naturals

Publishers == {1, 2}
VARIABLES pc, manifestDisk, candidates, newest, durableSuper, released, pin
vars == <<pc, manifestDisk, candidates, newest, durableSuper, released, pin>>

Init ==
  /\ pc = [p \in Publishers |-> "write"]
  /\ manifestDisk = {0}
  /\ candidates = {0} /\ newest = 0 /\ durableSuper = 0
  /\ released = {} /\ pin = 0

\* Each job's manifest write is a separate IO from its durable flush.
ManifestWrite(p) ==
  /\ pc[p] = "write"
  /\ pc' = [pc EXCEPT ![p] = "sync"]
  /\ UNCHANGED <<manifestDisk, candidates, newest, durableSuper, released, pin>>
ManifestSync(p) ==
  /\ pc[p] = "sync"
  /\ manifestDisk' = manifestDisk \cup {p}
  /\ pc' = [pc EXCEPT ![p] = "super"]
  /\ UNCHANGED <<candidates, newest, durableSuper, released, pin>>

\* lockSuper is held across write and sync. A stale job does not overwrite a
\* newer superblock; a newer job cannot enter while a superblock awaits sync.
SuperWrite(p) ==
  /\ pc[p] = "super"
  /\ \A q \in Publishers : pc[q] # "superSync"
  /\ IF p > newest
     THEN /\ newest' = p /\ candidates' = candidates \cup {p}
          /\ pc' = [pc EXCEPT ![p] = "superSync"]
     ELSE /\ pc' = [pc EXCEPT ![p] = "done"]
          /\ UNCHANGED <<newest, candidates>>
  /\ UNCHANGED <<manifestDisk, durableSuper, released, pin>>
SuperSync(p) ==
  /\ pc[p] = "superSync"
  /\ candidates' = {p} /\ durableSuper' = p
  /\ pc' = [pc EXCEPT ![p] = "done"]
  /\ UNCHANGED <<manifestDisk, newest, released, pin>>

\* An open reader initially pins manifest zero; discarding it permits reuse.
Discard ==
  /\ pin = 0 /\ pin' = 2
  /\ UNCHANGED <<pc, manifestDisk, candidates, newest, durableSuper, released>>

\* Each old run/manifest becomes pending at the following publication's seq.
\* releaseStep requires both a sufficiently new durable super and every pin.
Release(c) ==
  /\ c \in {0, 1} /\ c \notin released
  /\ durableSuper > c /\ pin > c
  /\ released' = released \cup {c}
  /\ UNCHANGED <<pc, manifestDisk, candidates, newest, durableSuper, pin>>
Next ==
  \/ \E p \in Publishers : ManifestWrite(p) \/ ManifestSync(p) \/ SuperWrite(p) \/ SuperSync(p)
  \/ \E c \in {0, 1} : Release(c)
  \/ Discard \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
  /\ \A p \in Publishers : WF_vars(ManifestWrite(p)) /\ WF_vars(ManifestSync(p))
       /\ WF_vars(SuperWrite(p)) /\ WF_vars(SuperSync(p))
  /\ WF_vars(Discard) /\ \A c \in {0, 1} : WF_vars(Release(c))

CrashReadsManifest == \A s \in candidates : s \in manifestDisk /\ s \notin released
SnapshotReads == pin \notin released
SuperNeverRegresses == durableSuper <= newest
ReleaseCompletes == <>({0, 1} \subseteq released)
====
