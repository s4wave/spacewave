---- MODULE Transfer ----
\* A Space transfer: Transfer.Execute in transfer.go and the session routine
\* that runs it, StartTransfer in core/resource/session/transfer.go.
\*
\* The person picks a mode and the Spaces to move. Execute scans the source;
\* Merge and Migrate, which delete the source, refuse a pick that leaves a Space
\* out. It decides each picked Space from what the target holds, then moves it
\* or records it as not moved, and continues with the next. Migrate signs the
\* target Spaces with the source's peer key, so it copies that key to the
\* target next. Merge and Migrate then delete each moved Space, every session
\* of the source account, and last the source volume. The routine can stop at
\* any step, by a crash or a failed copy, and the person can start it
\* again, with any mode and pick; nothing a run keeps on the source decides
\* anything, so every run asks the target about every picked Space.
\*
\* The target keeps, under each Space it lists for a transfer, the Space's
\* origin: ADDED when the transfer listed it with an empty genesis, WRITTEN
\* with the source head it wrote. The source head names the replayed World; a
\* target state is unedited while its World is still the one the origin names.
\* A delete on the target removes the origin with the Space. Per Space:
\*
\*   not listed                                 add, copy, hold, check, write
\*   ADDED, target state the empty genesis      copy, hold, check, write
\*   WRITTEN at the source's current head       moved, no write
\*   WRITTEN, source moved, target unedited     copy the newer source over it
\*   anything else                              not moved: no write
\*
\* A Space the source holds as a member of someone else's shared object
\* ("shared") is never copied, since a copy would start a new object without
\* its other members. When the target is a member of the same object, the
\* Space is moved with no write, but only from a local source: a local delete
\* removes this account's copy and nothing of the object, while a cloud delete
\* deletes the object for every member. Otherwise it is not moved.
\*
\* The write is a compare and set on the target state the decision saw, so a
\* write on the target during the copy makes the Space not moved. A run with
\* any Space not moved ends with that list and cleans up nothing. Cleanup
\* checks each Space again just before deleting it; a Space whose source moved
\* since its copy is moved again first. The delete also needs the target to
\* hold the Space as moved; one the target no longer lists as written there,
\* such as one deleted on the target since, stays on the source, not moved.
\*
\* A cloud target stores no origin. It reads a listed Space with no config as
\* arriving, and a configured one as moved only when its World is the
\* source's current World; it never writes a configured Space, so a Space it
\* holds from an earlier run is left not moved once the source moves on
\* (CloudLimit).
\*
\* The person may write to the source at any time except during cleanup, and
\* may write to, delete or create a Space on the target at any time, or change
\* its membership, such as a grant to a second device or an invite, which
\* leaves its World alone. A write over a changed membership would drop it. The code
\* checks the source head just before each delete, so a source write between
\* that check and the delete is the one window the model leaves out.
\*
\* The source account may have several sessions, and the person may start a
\* run from any session still listed. A run started from a session whose
\* volume is gone remounts it empty, under a new peer key. While a session is
\* listed the person may create a Space on the source, NewSpace, which no run
\* picked; the volume delete lists the source again and keeps the volume, and
\* fails the run, while it holds any Space.
\*
\* The target's collector frees a copied World at any time once neither the
\* stage nor the import root holds it. A stall past the stage's lease ends the
\* stage early. Adding a Space the target does not list releases every root its
\* bucket holds, as a create of an abandoned id does, so the hold follows the
\* add.

EXTENDS Naturals, FiniteSets

\* The initial choices a config checks, each a nonempty subset of BOOLEAN:
\* the kind of target and of source (TRUE for a cloud account), and whether
\* the source holds Space a, and Space b, as a member of another's shared
\* object. Each is chosen at Init and never changes, so the configs that split
\* them check, together, exactly the behaviors of the model with all four set
\* to BOOLEAN.
CONSTANTS TgtCloud, SrcCloud, SharedA, SharedB

Spaces == {"a", "b"}
\* A Space the person creates on the source during a run, under a new id.
NewSpace == "c"
Modes == {"merge", "migrate", "mirror"}
\* The source account's sessions.
Sessions == {"s1", "s2"}
\* MaxRuns bounds the runs the person starts after a run ends.
MaxRuns == 1
\* MaxEdits bounds the writes to each source Space.
MaxEdits == 1
\* MaxTouches bounds the person's writes, deletes, creates and membership
\* changes on the target.
MaxTouches == 2
\* MaxFaults bounds the crashes, failures and stage lapses in one behavior, so
\* the model is finite and a transfer that stops faulting can finish.
MaxFaults == 1

\* A target state: none (not listed), the empty genesis a transfer lists, a
\* state written from source version v and edited e times since, or a state
\* the transfer did not write; c counts the membership changes since.
None == [k |-> "none", v |-> 0, e |-> 0, c |-> 0]
Empty == [k |-> "empty", v |-> 0, e |-> 0, c |-> 0]
Src(v) == [k |-> "src", v |-> v, e |-> 0, c |-> 0]
Other == [k |-> "other", v |-> 0, e |-> 0, c |-> 0]
\* The target is a member of the same shared object as the source.
Member == [k |-> "member", v |-> 0, e |-> 0, c |-> 0]
\* An origin: none, ADDED, or WRITTEN at source head h.
NoOrigin == [ph |-> "none", h |-> 0]
Added == [ph |-> "added", h |-> 0]
Written(h) == [ph |-> "written", h |-> h]

VARIABLES
  mode,     \* the requested mode
  picked,   \* the Spaces the person picked in the wizard
  pc,       \* the routine's step; "idle" when no routine runs
  entries,  \* the Spaces this run transfers, from its scan
  cur,      \* the Space this run is moving, or "none"
  step,     \* cur's next step
  expect,   \* the target state cur's write expects to replace
  cv,       \* the source version this run copied for cur
  moved,    \* the Spaces this run moved
  notMoved, \* the Spaces this run left not moved
  staged,   \* the Spaces whose copied World this run's open stage holds
  blocks,   \* the Spaces whose World blocks the target stores
  held,     \* the Spaces whose World the target holds
  tc,       \* each Space's target state
  origin,   \* each Space's origin on the target
  src,      \* the Spaces the source volume holds
  ver,      \* each source Space's head, raised by a source write
  delOk,    \* each deleted source Space was on the target from its head
  lost,     \* a write replaced an edited or foreign target state
  touched,  \* the Spaces the person wrote, deleted, created or changed on the target
  touches,
  cloudTgt, \* the target is a cloud account, which stores no origin
  unjust,   \* a run left a Space not moved that the person did not change
  runs,
  vol,      \* the source volume exists
  sessions, \* the source account's listed sessions
  key,      \* the target holds the source's peer key
  fresh,    \* the source volume was remounted empty under a new peer key
  faults,
  shared,   \* the source Spaces held as a member of another's shared object
  cloudSrc, \* the source is a cloud account
  alive,    \* the shared objects a second member still reads
  keyLost,  \* the target lacks the source's peer key after a Migrate deleted the
            \* source volume, or holds a new key copied over it
  newLost   \* the volume delete removed a Space the source created during a run
run == <<pc, entries, cur, step, expect, cv, moved, notMoved, staged>>
target == <<blocks, held, tc, origin, lost, touched, touches, cloudTgt, unjust>>
source == <<src, ver, delOk, vol, sessions, key, fresh, shared, cloudSrc, alive, keyLost, newLost>>
vars == <<mode, picked, run, target, source, runs, faults>>

Deletes == mode \in {"merge", "migrate"}
Ends == {"done", "notmoved", "refused"}
Listed(s) == tc[s].k # "none"

Init ==
  /\ mode \in Modes
  /\ picked \in SUBSET Spaces \ {{}}
  /\ pc = "idle"
  /\ entries = {}
  /\ cur = "none"
  /\ step = "none"
  /\ expect = None
  /\ cv = 0
  /\ moved = {}
  /\ notMoved = {}
  /\ staged = {}
  \* The target may already list a Space the transfer did not write.
  /\ shared \in {S \in SUBSET Spaces : ("a" \in S) \in SharedA /\ ("b" \in S) \in SharedB}
  /\ cloudSrc \in SrcCloud
  /\ alive = shared
  /\ tc \in [Spaces -> {None, Other, Member}]
  /\ blocks = {s \in Spaces : Listed(s)}
  /\ held = blocks
  /\ origin = [s \in Spaces |-> NoOrigin]
  /\ lost = FALSE
  /\ touched = [s \in Spaces |-> Listed(s)]
  /\ touches = 0
  /\ cloudTgt \in TgtCloud
  /\ unjust = FALSE
  /\ runs = 0
  /\ src = Spaces
  /\ ver = [s \in Spaces |-> 0]
  /\ delOk = [s \in Spaces |-> TRUE]
  /\ vol = TRUE
  /\ sessions = Sessions
  /\ key = FALSE
  /\ fresh = FALSE
  /\ keyLost = FALSE
  /\ newLost = FALSE
  /\ faults = 0

\* Clear this run's memory and go to pc.
Stop(to) ==
  /\ pc' = to /\ entries' = {} /\ cur' = "none" /\ step' = "none"
  /\ expect' = None /\ cv' = 0 /\ moved' = {} /\ notMoved' = {} /\ staged' = {}

\* StartTransfer builds the source from a listed session's volume. A volume
\* already deleted mounts again, empty and with a new peer key.
Start ==
  /\ pc = "idle" /\ sessions # {}
  /\ ~(Deletes /\ src \ picked # {})
  /\ pc' = "spaces"
  /\ entries' = src \cap picked
  /\ IF vol THEN UNCHANGED fresh ELSE fresh' = TRUE
  /\ vol' = TRUE
  /\ UNCHANGED <<mode, picked, cur, step, expect, cv, moved, notMoved, staged, target, src, ver, delOk, sessions, key, shared, cloudSrc, alive, keyLost, newLost, runs, faults>>

\* Merge and Migrate refuse to start when the pick leaves out a Space, since
\* deleting the volume would delete it.
Refuse ==
  /\ pc = "idle" /\ sessions # {} /\ vol
  /\ Deletes /\ src \ picked # {}
  /\ Stop("refused")
  /\ UNCHANGED <<mode, picked, target, source, runs, faults>>

\* Take the next Space this run has not decided.
Begin(s) ==
  /\ pc = "spaces" /\ cur = "none" /\ s \in entries \ (moved \cup notMoved)
  /\ cur' = s /\ step' = "decide"
  /\ UNCHANGED <<mode, picked, pc, entries, expect, cv, moved, notMoved, staged, target, source, runs, faults>>

\* A local target reads the origin it stores; a cloud target stores none and
\* reads the Space's config and World alone. An arriving Space holds the empty
\* genesis.
Arriving(t, o) == t = Empty /\ (cloudTgt \/ o = Added)
\* The target holds the source's current World, whatever else changed, so no
\* write follows.
AtSourceHead(t, o) == (cloudTgt \/ o.ph = "written") /\ t.k = "src" /\ t.v = ver[cur] /\ t.e = 0
\* A local target holds a state this source wrote and nothing changed since,
\* so a write replaces only the World the transfer wrote.
Rewritable(t, o) == ~cloudTgt /\ o.ph = "written" /\ t = Src(o.h)
\* CloudLimit is a limit of the cloud target, not a choice. It stores no
\* origin, so it cannot tell an earlier copy this source wrote from a Space
\* someone else wrote, and leaves both alone. A retry after a fault, or a
\* second Merge or Mirror, whose source moved on since the cloud copy was
\* written therefore leaves the Space not moved although the person changed
\* nothing. No data is lost: the cloud keeps its older copy, and the source
\* keeps the Space, since a run with a Space not moved cleans up nothing.
\* NotMovedJustified accepts this case and no other; delete CloudLimit and
\* the cloud case of Decide when the cloud target stores the origin.
CloudLimit(s) == cloudTgt /\ tc[s].k = "src" /\ tc[s].v < ver[s]

\* Decide the Space from its target state, its origin and the source head.
\* Listing an id the target does not list releases every root of its bucket.
\* A Space left not moved that the person did not change, or no longer
\* listed, is unjust.
Decide ==
  /\ pc = "spaces" /\ step = "decide"
  /\ LET t == tc[cur]
         o == origin[cur]
     IN CASE cur \in shared ->
               /\ IF t = Member /\ ~cloudSrc
                    THEN moved' = moved \cup {cur} /\ UNCHANGED notMoved
                    ELSE notMoved' = notMoved \cup {cur} /\ UNCHANGED moved
               /\ cur' = "none" /\ step' = "none"
               /\ UNCHANGED <<tc, origin, held, expect, unjust>>
          [] ~Listed(cur) ->
               /\ tc' = [tc EXCEPT ![cur] = Empty]
               /\ origin' = [origin EXCEPT ![cur] = Added]
               /\ held' = held \ {cur}
               /\ expect' = Empty /\ step' = "copy"
               /\ UNCHANGED <<cur, moved, notMoved, unjust>>
          [] Arriving(t, o) ->
               /\ expect' = t /\ step' = "copy"
               /\ UNCHANGED <<tc, origin, held, cur, moved, notMoved, unjust>>
          [] AtSourceHead(t, o) ->
               /\ moved' = moved \cup {cur}
               /\ cur' = "none" /\ step' = "none"
               /\ UNCHANGED <<tc, origin, held, expect, notMoved, unjust>>
          [] cloudTgt ->
               /\ notMoved' = notMoved \cup {cur}
               /\ unjust' = (unjust \/ ~(touched[cur] \/ CloudLimit(cur)))
               /\ cur' = "none" /\ step' = "none"
               /\ UNCHANGED <<tc, origin, held, expect, moved>>
          [] Rewritable(t, o) ->
               /\ expect' = t /\ step' = "copy"
               /\ UNCHANGED <<tc, origin, held, cur, moved, notMoved, unjust>>
          [] OTHER ->
               /\ notMoved' = notMoved \cup {cur}
               /\ unjust' = (unjust \/ ~(touched[cur] /\ Listed(cur)))
               /\ cur' = "none" /\ step' = "none"
               /\ UNCHANGED <<tc, origin, held, expect, moved>>
  /\ UNCHANGED <<mode, picked, pc, entries, cv, staged, blocks, lost, touched, touches, cloudTgt, source, runs, faults>>

\* Copy the Space's World at the source head under a new stage, which holds it
\* from its put.
Copy ==
  /\ pc = "spaces" /\ step = "copy"
  /\ blocks' = blocks \cup {cur}
  /\ staged' = {cur}
  /\ cv' = ver[cur]
  /\ step' = "hold"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, expect, moved, notMoved, held, tc, origin, lost, touched, touches, cloudTgt, unjust, source, runs, faults>>

\* Hold the World under the import root.
Hold ==
  /\ pc = "spaces" /\ step = "hold"
  /\ held' = held \cup {cur}
  /\ step' = "check"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, expect, cv, moved, notMoved, staged, blocks, tc, origin, lost, touched, touches, cloudTgt, unjust, source, runs, faults>>

\* Check the stage still held the whole copy until the hold; a lapsed stage
\* may have lost it, and the run fails so a retry copies again.
Check ==
  /\ pc = "spaces" /\ step = "check"
  /\ IF cur \in blocks
       THEN /\ step' = "write"
            /\ UNCHANGED <<pc, entries, cur, expect, cv, moved, notMoved, staged>>
       ELSE Stop("idle")
  /\ UNCHANGED <<mode, picked, target, source, runs, faults>>

\* Write the Space's state and its WRITTEN origin, only while the target state
\* is the one the decision saw; otherwise the Space is not moved.
Write ==
  /\ pc = "spaces" /\ step = "write"
  /\ IF tc[cur] = expect
       THEN /\ lost' = (lost \/ tc[cur].e > 0 \/ tc[cur].c > 0 \/ tc[cur].k = "other")
            /\ tc' = [tc EXCEPT ![cur] = Src(cv)]
            /\ origin' = [origin EXCEPT ![cur] = Written(cv)]
            /\ moved' = moved \cup {cur}
            /\ UNCHANGED notMoved
       ELSE /\ notMoved' = notMoved \cup {cur}
            /\ UNCHANGED <<lost, tc, origin, moved>>
  /\ step' = "release"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, expect, cv, staged, blocks, held, touched, touches, cloudTgt, unjust, source, runs, faults>>

\* Release the stage.
Release ==
  /\ pc = "spaces" /\ step = "release"
  /\ staged' = {}
  /\ cur' = "none" /\ step' = "none"
  /\ UNCHANGED <<mode, picked, pc, entries, expect, cv, moved, notMoved, target, source, runs, faults>>

\* Every Space is decided: a run with any Space not moved ends there, with
\* nothing cleaned up.
SpacesDone ==
  /\ pc = "spaces" /\ cur = "none" /\ entries \subseteq moved \cup notMoved
  /\ IF notMoved # {}
       THEN /\ pc' = "notmoved"
            /\ UNCHANGED <<entries, cur, step, expect, cv, moved, notMoved, staged>>
       ELSE /\ pc' = CASE mode = "migrate" -> "key" [] Deletes -> "cleanup" [] OTHER -> "done"
            /\ UNCHANGED <<entries, cur, step, expect, cv, moved, notMoved, staged>>
  /\ UNCHANGED <<mode, picked, target, source, runs, faults>>

\* Migrate gives the target the source's peer key while the source holds it.
\* A remounted volume's new key replaces the one the target's Spaces are
\* signed with.
CopyKey ==
  /\ pc = "key"
  /\ key' = TRUE
  /\ keyLost' = (keyLost \/ fresh)
  /\ pc' = "cleanup"
  /\ UNCHANGED <<mode, picked, entries, cur, step, expect, cv, moved, notMoved, staged, target, src, ver, delOk, vol, sessions, fresh, runs, faults, shared, cloudSrc, alive, newLost>>

\* The target holds the Space as written from its current source head, or is
\* a member of the shared object a local source holds.
AtHead(s) ==
  IF s \in shared THEN tc[s] = Member /\ ~cloudSrc ELSE origin[s] = Written(ver[s])

\* Cleanup checks each Space just before deleting it from the source.
DeleteSpace(s) ==
  /\ pc = "cleanup" /\ s \in entries \cap src /\ AtHead(s)
  /\ src' = src \ {s}
  /\ delOk' = [delOk EXCEPT ![s] =
       IF s \in shared THEN tc[s] = Member ELSE tc[s].k = "src" /\ tc[s].v = ver[s]]
  /\ alive' = IF s \in shared /\ cloudSrc THEN alive \ {s} ELSE alive
  /\ UNCHANGED <<mode, picked, run, target, ver, vol, sessions, key, fresh, runs, faults, shared, cloudSrc, keyLost, newLost>>

\* A Space whose source moved since its copy goes back to be decided again.
Recheck(s) ==
  /\ pc = "cleanup" /\ s \in entries \cap src /\ ~AtHead(s) /\ origin[s].ph = "written"
  /\ pc' = "spaces"
  /\ moved' = moved \ {s}
  /\ UNCHANGED <<mode, picked, entries, cur, step, expect, cv, notMoved, staged, target, source, runs, faults>>

\* A Space the target no longer holds as moved stays on the source, and the
\* run ends with it not moved. When the source also moved, the code may move
\* it again instead, as Recheck does.
TargetLost(s) ==
  /\ pc = "cleanup" /\ s \in entries \cap src /\ ~AtHead(s)
  /\ (s \in shared \/ origin[s].ph # "written")
  /\ pc' = "notmoved"
  /\ moved' = moved \ {s} /\ notMoved' = notMoved \cup {s}
  /\ UNCHANGED <<mode, picked, entries, cur, step, expect, cv, staged, target, source, runs, faults>>

\* Then it deletes every session of the source account, after every Space.
DeleteSession(x) ==
  /\ pc = "cleanup" /\ entries \cap src = {} /\ x \in sessions
  /\ sessions' = sessions \ {x}
  /\ UNCHANGED <<mode, picked, run, target, src, ver, delOk, vol, key, fresh, runs, faults, shared, cloudSrc, alive, keyLost, newLost>>

\* Last it deletes the source volume, once no session lists it and it holds
\* no Space.
DeleteVolume ==
  /\ pc = "cleanup" /\ entries \cap src = {} /\ sessions = {}
  /\ src = {}
  /\ vol' = FALSE /\ src' = {}
  /\ alive' = IF cloudSrc THEN alive \ (src \cap shared) ELSE alive
  /\ keyLost' = (keyLost \/ (mode = "migrate" /\ ~key))
  /\ newLost' = (newLost \/ NewSpace \in src)
  /\ pc' = "done"
  /\ UNCHANGED <<mode, picked, entries, cur, step, expect, cv, moved, notMoved, staged, target, ver, delOk, sessions, key, fresh, runs, faults, shared, cloudSrc>>

\* The volume delete finds a Space the run did not move, keeps the volume and
\* fails the run; the sessions already deleted leave it Stranded.
KeepVolume ==
  /\ pc = "cleanup" /\ entries \cap src = {} /\ sessions = {} /\ src # {}
  /\ Stop("idle")
  /\ UNCHANGED <<mode, picked, target, source, runs, faults>>

\* The routine stops at any step, by a crash, a cancel or a failed copy,
\* losing what it held in memory and ending its stage. The person may start
\* again with another mode and pick.
Fault ==
  /\ pc \notin Ends \cup {"idle"} /\ faults < MaxFaults
  /\ Stop("idle")
  /\ mode' \in Modes
  /\ picked' \in SUBSET Spaces \ {{}}
  /\ faults' = faults + 1
  /\ UNCHANGED <<target, source, runs>>

\* The routine stalls past its stage's lease, and the stage stops holding.
Lapse ==
  /\ staged # {} /\ faults < MaxFaults
  /\ staged' = {}
  /\ faults' = faults + 1
  /\ UNCHANGED <<mode, picked, pc, entries, cur, step, expect, cv, moved, notMoved, target, source, runs>>

\* The target's collector frees a World nothing holds.
Free(s) ==
  /\ s \in blocks \ (staged \cup held)
  /\ blocks' = blocks \ {s}
  /\ UNCHANGED <<mode, picked, run, held, tc, origin, lost, touched, touches, cloudTgt, unjust, source, runs, faults>>

\* The person creates a Space on the source through a listed session.
SourceCreate ==
  /\ vol /\ sessions # {} /\ NewSpace \notin src
  /\ src' = src \cup {NewSpace}
  /\ UNCHANGED <<mode, picked, run, target, ver, delOk, vol, sessions, key, fresh, runs, faults, shared, cloudSrc, alive, keyLost, newLost>>

\* The person writes to a source Space, except during cleanup.
SourceWrite(s) ==
  /\ s \in src /\ ver[s] < MaxEdits /\ pc # "cleanup"
  /\ ver' = [ver EXCEPT ![s] = @ + 1]
  /\ UNCHANGED <<mode, picked, run, target, src, delOk, vol, sessions, key, fresh, runs, faults, shared, cloudSrc, alive, keyLost, newLost>>

\* The person writes to a Space the target lists; the target holds its World.
\* A Space the transfer added and has not written is arriving: the target
\* refuses to mount it, so nothing else writes it.
TargetWrite(s) ==
  /\ Listed(s) /\ touches < MaxTouches /\ origin[s] # Added
  /\ tc' = [tc EXCEPT ![s] = CASE @.k = "src" -> [@ EXCEPT !.e = @ + 1]
                                 [] @.k = "member" -> @
                                 [] OTHER -> [Other EXCEPT !.e = 1]]
  /\ held' = held \cup {s} /\ blocks' = blocks \cup {s}
  /\ touched' = [touched EXCEPT ![s] = TRUE] /\ touches' = touches + 1
  /\ UNCHANGED <<mode, picked, run, origin, lost, cloudTgt, unjust, source, runs, faults>>

\* The person changes the membership of a Space the target lists and holds as
\* its own, leaving its World alone.
TargetConfig(s) ==
  /\ tc[s].k \in {"src", "other"} /\ touches < MaxTouches
  /\ tc' = [tc EXCEPT ![s].c = @ + 1]
  /\ touched' = [touched EXCEPT ![s] = TRUE] /\ touches' = touches + 1
  /\ UNCHANGED <<mode, picked, run, blocks, held, origin, lost, cloudTgt, unjust, source, runs, faults>>

\* The person deletes a Space on the target, its origin with it.
TargetDelete(s) ==
  /\ Listed(s) /\ touches < MaxTouches
  /\ tc' = [tc EXCEPT ![s] = None]
  /\ origin' = [origin EXCEPT ![s] = NoOrigin]
  /\ held' = held \ {s}
  /\ touched' = [touched EXCEPT ![s] = TRUE] /\ touches' = touches + 1
  /\ UNCHANGED <<mode, picked, run, blocks, lost, cloudTgt, unjust, source, runs, faults>>

\* The person creates a Space on the target with an id the target does not
\* list.
TargetCreate(s) ==
  /\ ~Listed(s) /\ touches < MaxTouches
  /\ tc' = [tc EXCEPT ![s] = Other]
  /\ held' = held \cup {s} /\ blocks' = blocks \cup {s}
  /\ touched' = [touched EXCEPT ![s] = TRUE] /\ touches' = touches + 1
  /\ UNCHANGED <<mode, picked, run, origin, lost, cloudTgt, unjust, source, runs, faults>>

\* After a run ends, the person starts another from a listed session of the
\* same source to the same target, with any mode and pick.
NewRun ==
  /\ pc \in Ends /\ sessions # {} /\ runs < MaxRuns
  /\ Stop("idle")
  /\ mode' \in Modes
  /\ picked' \in SUBSET Spaces \ {{}}
  /\ runs' = runs + 1
  /\ UNCHANGED <<target, source, faults>>

\* A run stopped after deleting the account's last session: no session is
\* left to start a run from, and the volume, if the stop came before its
\* delete, stays on disk unlisted.
Stranded == pc = "idle" /\ sessions = {}

Done == (pc \in Ends \/ Stranded) /\ UNCHANGED vars

Progress ==
  \/ Start \/ Refuse \/ Decide \/ Copy \/ Hold \/ Check \/ Write \/ Release
  \/ SpacesDone \/ CopyKey \/ DeleteVolume \/ KeepVolume
  \/ \E x \in Sessions : DeleteSession(x)
  \/ \E s \in Spaces : Begin(s) \/ DeleteSpace(s) \/ Recheck(s) \/ TargetLost(s)
Next ==
  \/ Progress \/ Fault \/ Lapse \/ Done \/ NewRun \/ SourceCreate
  \/ \E s \in Spaces :
       Free(s) \/ SourceWrite(s) \/ TargetWrite(s) \/ TargetDelete(s) \/ TargetCreate(s)
       \/ TargetConfig(s)
Spec == Init /\ [][Next]_vars /\ WF_vars(Progress)

\* No write replaces a target state the person edited or the transfer did not
\* write.
NoEditOverwritten == ~lost
\* Cleanup deletes a source Space only when the target's state of it was
\* written from that source Space at the head it had when deleted.
CleanupAtHead == \A s \in Spaces \ src : delOk[s]
\* An arriving Space holds the empty genesis until the transfer writes it.
ArrivingUntouched == \A s \in Spaces : origin[s] = Added => tc[s] = Empty
\* A Space is left not moved only when the target lists a version the person
\* changed there, or it is a shared object the transfer must not copy, never
\* for a reason the transfer could resolve itself, except the cloud target's
\* named limit, CloudLimit. The excuse is judged when the run leaves the Space;
\* a Space the person deleted is moved again.
NotMovedJustified ==
  /\ ~unjust
  /\ \A s \in notMoved : touched[s] \/ s \in shared \/ CloudLimit(s)
\* A second member of a shared object the source holds keeps reading it.
MembersKeepShared == alive = shared
\* No Space the source holds as a member is ever written on the target.
SharedNotCopied == \A s \in shared : tc[s].k # "src"
\* A run that ends with a Space not moved keeps that Space, the source volume
\* and a source session. Cleanup may already have deleted Spaces it checked
\* at their head.
NotMovedKeepsSource == pc \in Ends /\ notMoved # {} => notMoved \subseteq src /\ vol /\ sessions # {}
\* The target never stores a written state whose World nothing holds or it
\* lost.
StateHeld == \A s \in Spaces : tc[s].k = "src" => s \in held /\ s \in blocks
\* Migrate deletes the source volume only after the target holds its peer key,
\* and no later run copies a different key over it.
MigrateKeepsKey == ~keyLost
\* The volume delete never removes a Space the run did not move.
NewSpaceKept == ~newLost
\* Every run ends: done, not moved, refused, or stranded.
Finishes == [](pc \notin Ends => <>(pc \in Ends \/ Stranded))
====
