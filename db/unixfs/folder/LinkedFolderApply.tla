---- MODULE LinkedFolderApply ----
\* One path in the owner's linked folder, and the owner's node applying a
\* member's remote change to it. The owner edits or deletes the file on disk at
\* any moment, also in place through an open handle. A member writes a new
\* version or deletes the file, each against the version the member last saw,
\* its base. The new version's bytes stay in the member's store until the node
\* fetches them (decision A: chunks stay where they were linked or written).
\*
\* The node applies a write in four steps: fetch the new version into a staged
\* file beside the path; check that the disk still holds the base; exchange the
\* staged file with the path atomically (renameat2 RENAME_EXCHANGE on Linux,
\* renamex_np RENAME_SWAP on macOS); then verify the displaced file. A
\* displaced base is the replaced version and goes; anything else, an edit that
\* raced the exchange, keeps a conflict name. When the owner deleted the file,
\* the member's version goes back at the path, since a modification is newer
\* intent than the deletion; the rename does not replace anything (renameat2
\* RENAME_NOREPLACE, renamex_np RENAME_EXCL), so a file the owner put at the
\* path after the check makes it fail and the member's version takes a conflict
\* name. A disk holding any other version gives the member's version the
\* conflict name and leaves the disk's file at its path.
\* A delete is checked the same way, moved to the trash, and verified there: a
\* file edited after the check comes back to its path without replacing
\* anything (renameat2 RENAME_NOREPLACE, renamex_np RENAME_EXCL), or takes a
\* conflict name when the owner has put a new file at the path meanwhile.
\*
\* Versions are numbered 1..MaxVer and each is written once; 0 means none.
\* A member makes one change in a behavior: changes to one path apply one at a
\* time, and each starts from the same idle state, so one covers them all.
EXTENDS Naturals
CONSTANT MaxVer

VARIABLES
  disk,           \* the version at the path, or 0 when the path is empty
  applied,        \* the version the node last indexed or wrote at the path
  next,           \* the next unused version number
  pending,        \* the member's change the node is applying
  phase,          \* the node's step in applying it
  held,           \* the displaced file: beside the path, or in the trash
  copies,         \* versions under a conflict name in the folder
  trash,          \* versions in the owner's trash
  ownerReplaced,  \* versions the owner's own edits and deletes replaced
  remoteReplaced, \* versions a member's applied write replaced
  everOwned,      \* ghost: every version the folder's path or a held file had
  memberWrites,   \* ghost: every version a member wrote
  remoteBases,    \* ghost: the base of every member write
  deleteBases     \* ghost: the base of every member delete
vars == <<disk, applied, next, pending, phase, held, copies, trash,
          ownerReplaced, remoteReplaced, everOwned, memberWrites, remoteBases,
          deleteBases>>

NoChange == [kind |-> "none", base |-> 0, new |-> 0]
Kinds == {"none", "write", "delete"}
Phases == {"idle", "staged", "checked", "restoring", "swapped", "trashed"}

Init ==
  /\ disk = 1 /\ applied = 1 /\ next = 2
  /\ pending = NoChange /\ phase = "idle" /\ held = 0
  /\ copies = {} /\ trash = {}
  /\ ownerReplaced = {} /\ remoteReplaced = {}
  /\ everOwned = {1} /\ memberWrites = {} /\ remoteBases = {} /\ deleteBases = {}

\* Put v at the path.
Put(v) == disk' = v /\ everOwned' = everOwned \cup ({v} \ {0})
\* The node finishes the change.
Done == phase' = "idle" /\ pending' = NoChange
\* What a member's change leaves alone.
Ghosts == UNCHANGED <<memberWrites, remoteBases, deleteBases>>

\* The owner's side.
OwnerEdit == next <= MaxVer
  /\ Put(next) /\ next' = next + 1
  /\ ownerReplaced' = ownerReplaced \cup ({disk} \ {0})
  /\ UNCHANGED <<applied, pending, phase, held, copies, trash, remoteReplaced>>
  /\ Ghosts
OwnerDelete == disk # 0
  /\ Put(0) /\ ownerReplaced' = ownerReplaced \cup {disk}
  /\ UNCHANGED <<applied, next, pending, phase, held, copies, trash, remoteReplaced>>
  /\ Ghosts
\* An editor still holding the displaced file open writes into it.
EditHeld == phase \in {"swapped", "trashed"} /\ held # 0 /\ next <= MaxVer
  /\ held' = next /\ next' = next + 1
  /\ ownerReplaced' = ownerReplaced \cup {held}
  /\ everOwned' = everOwned \cup {next}
  /\ UNCHANGED <<disk, applied, pending, phase, copies, trash, remoteReplaced>>
  /\ Ghosts
\* The node indexes the file on disk and publishes it.
Index == phase = "idle" /\ disk # 0 /\ applied # disk
  /\ applied' = disk
  /\ UNCHANGED <<disk, next, pending, phase, held, copies, trash,
                 ownerReplaced, remoteReplaced, everOwned>>
  /\ Ghosts

\* A member's side: one change, against the published version.
Asked == remoteBases \cup deleteBases # {}
MemberWrite == ~Asked /\ applied # 0 /\ next <= MaxVer
  /\ pending' = [kind |-> "write", base |-> applied, new |-> next]
  /\ next' = next + 1
  /\ memberWrites' = memberWrites \cup {next}
  /\ remoteBases' = remoteBases \cup {applied}
  /\ UNCHANGED <<disk, applied, phase, held, copies, trash,
                 ownerReplaced, remoteReplaced, everOwned, deleteBases>>
MemberDelete == ~Asked /\ applied # 0
  /\ pending' = [kind |-> "delete", base |-> applied, new |-> 0]
  /\ deleteBases' = deleteBases \cup {applied}
  /\ UNCHANGED <<disk, applied, next, phase, held, copies, trash,
                 ownerReplaced, remoteReplaced, everOwned, memberWrites,
                 remoteBases>>

\* The node applies a write.
Fetch == phase = "idle" /\ pending.kind = "write"
  /\ phase' = "staged"
  /\ UNCHANGED <<disk, applied, next, pending, held, copies, trash,
                 ownerReplaced, remoteReplaced, everOwned>>
  /\ Ghosts
CheckWrite == phase = "staged"
  /\ (IF disk = pending.base
       THEN phase' = "checked" /\ UNCHANGED <<pending, copies>>
       ELSE IF disk = 0
         THEN phase' = "restoring" /\ UNCHANGED <<pending, copies>>
       ELSE copies' = copies \cup {pending.new} /\ Done)
  /\ UNCHANGED <<disk, applied, next, held, trash, ownerReplaced,
                 remoteReplaced, everOwned>>
  /\ Ghosts
\* The owner deleted the file: put the member's version back at the path
\* without replacing anything, or under a conflict name when the owner put a
\* new file there after the check.
Restore == phase = "restoring"
  /\ (IF disk = 0
       THEN Put(pending.new) /\ applied' = pending.new /\ UNCHANGED copies
       ELSE copies' = copies \cup {pending.new}
              /\ UNCHANGED <<disk, applied, everOwned>>)
  /\ Done
  /\ UNCHANGED <<next, held, trash, ownerReplaced, remoteReplaced>>
  /\ Ghosts
Exchange == phase = "checked" /\ pending.kind = "write"
  /\ held' = disk /\ Put(pending.new) /\ phase' = "swapped"
  /\ UNCHANGED <<applied, next, pending, copies, trash, ownerReplaced,
                 remoteReplaced>>
  /\ Ghosts
VerifyWrite == phase = "swapped"
  /\ (IF held = pending.base
       THEN remoteReplaced' = remoteReplaced \cup {held} /\ UNCHANGED copies
       ELSE copies' = copies \cup ({held} \ {0}) /\ UNCHANGED remoteReplaced)
  /\ held' = 0 /\ applied' = pending.new /\ Done
  /\ UNCHANGED <<disk, next, trash, ownerReplaced, everOwned>>
  /\ Ghosts

\* The node applies a delete. A disk that moved on keeps its file.
CheckDelete == phase = "idle" /\ pending.kind = "delete"
  /\ (IF disk = pending.base
       THEN phase' = "checked" /\ UNCHANGED pending
       ELSE Done)
  /\ UNCHANGED <<disk, applied, next, held, copies, trash, ownerReplaced,
                 remoteReplaced, everOwned>>
  /\ Ghosts
MoveToTrash == phase = "checked" /\ pending.kind = "delete"
  /\ held' = disk /\ Put(0) /\ phase' = "trashed"
  /\ UNCHANGED <<applied, next, pending, copies, trash, ownerReplaced,
                 remoteReplaced>>
  /\ Ghosts
VerifyDelete == phase = "trashed"
  /\ (IF held = pending.base
       THEN trash' = trash \cup {held} /\ UNCHANGED <<disk, copies, everOwned>>
       ELSE IF held = 0
         THEN UNCHANGED <<disk, trash, copies, everOwned>>
       ELSE IF disk = 0
         THEN Put(held) /\ UNCHANGED <<trash, copies>>
         ELSE copies' = copies \cup {held} /\ UNCHANGED <<disk, trash, everOwned>>)
  /\ held' = 0 /\ Done
  /\ UNCHANGED <<applied, next, ownerReplaced, remoteReplaced>>
  /\ Ghosts

\* The member's change is applied and the owner is done editing: the end.
Rest == Asked /\ pending = NoChange /\ UNCHANGED vars

Next == Rest \/ OwnerEdit \/ OwnerDelete \/ EditHeld \/ Index
  \/ MemberWrite \/ MemberDelete
  \/ Fetch \/ CheckWrite \/ Restore \/ Exchange \/ VerifyWrite
  \/ CheckDelete \/ MoveToTrash \/ VerifyDelete

TypeOK ==
  /\ disk \in 0..MaxVer /\ applied \in 0..MaxVer /\ next \in 1..(MaxVer + 1)
  /\ pending.kind \in Kinds /\ pending.base \in 0..MaxVer /\ pending.new \in 0..MaxVer
  /\ phase \in Phases /\ held \in 0..MaxVer
  /\ copies \subseteq 1..MaxVer /\ trash \subseteq 1..MaxVer
  /\ ownerReplaced \subseteq 1..MaxVer /\ remoteReplaced \subseteq 1..MaxVer

\* Every version the folder had is still at the path, under a conflict name,
\* in the trash or displaced and being verified, unless the owner's own edit or
\* a member's write against exactly that version replaced it.
NoLostVersion ==
  everOwned \subseteq ({disk, held} \cup copies \cup trash \cup ownerReplaced
                       \cup remoteReplaced)
\* Every member's write reaches the folder, at the path or under a conflict
\* name, or is still being applied.
MemberWriteKept ==
  memberWrites \subseteq (everOwned \cup copies \cup {pending.new})
\* A member's write replaces only the version that member saw.
RemoteReplacesOnlyBase == remoteReplaced \subseteq remoteBases
\* The trash holds only versions a member deleted, never an edit made after.
TrashOnlyDeleted == trash \subseteq deleteBases
\* A displaced file exists only while the node verifies it.
HeldOnlyWhileVerifying == held # 0 => phase \in {"swapped", "trashed"}
====
