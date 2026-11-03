---- MODULE BackfillSnapshot ----
\* A served engine can have a durable World before its background ownership
\* field is initialized. One read snapshot supplies BOTH root and World seqno.
\* Durable admission positions are not World seqnos: positions2 and3 have the
\* same World. A copy drains prior admissions outside the writer, retries a
\* changed read, and keeps its read/pin through publication. Teardown cancels
\* the copy before joining its read. Availability is the World page view:
\* protected kept pages PLUS the locally readable physical frontier, not a
\* claim that the current root block is physically stored. This models graphs
\* initially readable locally; peer holes are reported by the separate copy.
\* Retained-set consistency is out of scope.
EXTENDS Naturals
CONSTANTS CaptureHead, KeepPin, TrackRead, DrainDurable
VARIABLES admitted, done, retained, available, phase, root, seqno, pin, alive, closed, target
vars == <<admitted, done, retained, available, phase, root, seqno, pin, alive, closed, target>>
World(pos) == IF pos = 1 THEN 1 ELSE 2
Seq(r) == IF r = 1 THEN 7 ELSE IF r = 2 THEN 8 ELSE 0
Active == phase \in {"read", "copy", "copied", "published"}
Init == /\ admitted = 1 /\ done = 1 /\ retained = 0 /\ available = {1}
        /\ phase = "idle" /\ root = 0 /\ seqno = 0 /\ pin = 0
        /\ alive = TRUE /\ closed = FALSE /\ target = 1
Admit == /\ alive /\ admitted < 3 /\ admitted' = admitted + 1
         /\ UNCHANGED <<done, retained, available, phase, root, seqno, pin, alive, closed, target>>
Promote == /\ alive /\ done < admitted /\ done' = done + 1
           /\ available' = available \cup {World(done + 1)}
           /\ UNCHANGED <<admitted, retained, phase, root, seqno, pin, alive, closed, target>>
Hold == /\ alive /\ retained # World(done) /\ retained' = World(done)
        /\ UNCHANGED <<admitted, done, available, phase, root, seqno, pin, alive, closed, target>>
Read == /\ alive /\ phase = "idle"
        /\ root' = (IF CaptureHead THEN World(done) ELSE retained)
        /\ seqno' = Seq(IF CaptureHead THEN World(done) ELSE World(admitted))
        /\ pin' = (IF CaptureHead THEN World(done) ELSE 0) /\ phase' = "read"
        /\ UNCHANGED <<admitted, done, retained, available, alive, closed, target>>
Validate == /\ alive /\ phase = "read"
            /\ (~CaptureHead \/ ((~DrainDurable \/ done = admitted) /\ root = World(done)))
            /\ phase' = "copy" /\ pin' = (IF KeepPin THEN pin ELSE 0) /\ target' = admitted
            /\ UNCHANGED <<admitted, done, retained, available, root, seqno, alive, closed>>
Retry == /\ alive /\ CaptureHead /\ phase = "read"
         /\ ((DrainDurable /\ done # admitted) \/ root # World(done))
         /\ phase' = "wait" /\ pin' = 0
         /\ UNCHANGED <<admitted, done, retained, available, root, seqno, alive, closed, target>>
Wake == /\ alive /\ phase = "wait" /\ done = admitted /\ phase' = "idle"
        /\ UNCHANGED <<admitted, done, retained, available, root, seqno, pin, alive, closed, target>>
Copy == /\ alive /\ phase = "copy" /\ phase' = "copied"
        /\ UNCHANGED <<admitted, done, retained, available, root, seqno, pin, alive, closed, target>>
Publish == /\ alive /\ phase = "copied" /\ phase' = "published"
           /\ UNCHANGED <<admitted, done, retained, available, root, seqno, pin, alive, closed, target>>
Release == /\ phase = "published" /\ phase' = "finished" /\ pin' = 0
           /\ UNCHANGED <<admitted, done, retained, available, root, seqno, alive, closed, target>>
Sweep == /\ \E r \in available :
            /\ r # World(done) /\ r # pin /\ available' = available \ {r}
            /\ UNCHANGED <<admitted, done, retained, phase, root, seqno, pin, alive, closed, target>>
Retire == /\ alive /\ alive' = FALSE
          /\ UNCHANGED <<admitted, done, retained, available, phase, root, seqno, pin, closed, target>>
Cancel == /\ ~alive /\ phase # "finished" /\ phase' = "finished" /\ pin' = 0
          /\ UNCHANGED <<admitted, done, retained, available, root, seqno, alive, closed, target>>
Close == /\ ~alive /\ ~closed /\ (~TrackRead \/ ~Active)
         /\ closed' = TRUE
         /\ UNCHANGED <<admitted, done, retained, available, phase, root, seqno, pin, alive, target>>
Next == Admit \/ Promote \/ Hold \/ Read \/ Validate \/ Retry \/ Wake \/ Copy
        \/ Publish \/ Release \/ Sweep \/ Retire \/ Cancel \/ Close
        \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
CoherentHead == Active => root # 0 /\ seqno = Seq(root)
CopyReadable == phase \in {"copy", "copied", "published"} => root \in available
CoversDrainedTarget == phase \in {"copy", "copied", "published"} => root = World(target)
SnapshotLifetime == closed => ~Active
Released == phase = "finished" => pin = 0
====
