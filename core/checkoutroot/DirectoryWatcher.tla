-------------------------- MODULE DirectoryWatcher --------------------------
EXTENDS FiniteSets, Naturals

\* serve owns one source. FSCursorService opens a subscription for each active
\* directory cursor and releases it on cursor release or disconnect. Removal
\* retires the cursor. Overflow and source failure invalidate its listing.
\* The registry mutex linearizes subscribe, release, reconciliation and close.
\* Host changes precede dispatch and can interleave with a new subscription.
\* One Linux inode at two paths is enough to exercise shared registration.
\* Path-based backends use the resolved path as the native key.
\* Cursor status and invalidation here describe the watcher, before RPC delivery.
\* The kernel can drop a deleted inode before the reader dispatches its event.
\* A move notification may be lost and force reconciliation through overflow.
CONSTANT Cursors, Paths, Absent, Inode, ByInode
VARIABLES location, pending, source, status, path, registrations, kernel, dirty

vars == <<location, pending, source, status, path, registrations, kernel, dirty>>
Key(p) == IF ByInode THEN Inode ELSE p
Live == {c \in Cursors : status[c] = "live"}
Current == {c \in Live : path[c] = location}
Used(cs) == {Key(path[c]) : c \in cs}

Init ==
  /\ location \in Paths
  /\ pending = "none"
  /\ source \in {"available", "unavailable"}
  /\ status = [c \in Cursors |-> "idle"]
  /\ path = [c \in Cursors |-> Absent]
  /\ registrations = {}
  /\ kernel = FALSE
  /\ dirty = {}

Open(c) ==
  /\ source = "available"
  /\ status[c] = "idle"
  /\ location \in Paths
  /\ status' = [status EXCEPT ![c] = "live"]
  /\ path' = [path EXCEPT ![c] = location]
  /\ registrations' = registrations \cup {Key(location)}
  /\ kernel' = TRUE
  /\ UNCHANGED <<location, pending, source, dirty>>

OpenSnapshot(c) ==
  /\ (source # "closed")
  /\ status[c] = "idle"
  /\ location \in Paths
  /\ status' = [status EXCEPT ![c] = "snapshot"]
  /\ path' = [path EXCEPT ![c] = location]
  /\ UNCHANGED <<location, pending, source, registrations, kernel, dirty>>

Release(c) ==
  /\ (status[c] # "idle")
  /\ status' = [status EXCEPT ![c] = "idle"]
  /\ path' = [path EXCEPT ![c] = Absent]
  /\ dirty' = dirty \ {c}
  /\ LET remaining == Used(Live \ {c})
     IN /\ registrations' = registrations \cap remaining
        /\ kernel' = (kernel /\ registrations \subseteq remaining)
  /\ UNCHANGED <<location, pending, source>>

HostMove(to) ==
  /\ source = "available"
  /\ location \in Paths
  /\ to # location
  /\ location' = to
  /\ pending' \in IF kernel \/ (pending # "none") THEN {"move", "overflow"} ELSE {"none"}
  /\ kernel' = (kernel /\ to \in Paths)
  /\ UNCHANGED <<source, status, path, registrations, dirty>>

Reconcile ==
  /\ source = "available"
  /\ LET surviving == Current
     IN /\ status' = [c \in Cursors |->
                          IF c \in (Live \ surviving) THEN "retired" ELSE status[c]]
        /\ registrations' = Used(surviving)
        /\ kernel' = (surviving # {})
        /\ dirty' = (dirty \cup surviving) \ (Live \ surviving)
  /\ pending' = "none"
  /\ UNCHANGED <<location, source, path>>

DispatchMove == (pending = "move") /\ Reconcile

Refresh ==
  /\ source = "available"
  /\ dirty' = dirty \cup Live
  /\ UNCHANGED <<location, pending, source, status, path, registrations, kernel>>

Overflow == (pending # "move") /\ Reconcile

RearmUnavailable ==
  /\ source = "available"
  /\ Live # {}
  /\ LET surviving == Current
     IN /\ status' = [c \in Cursors |-> CASE c \in surviving -> "snapshot"
                                         [] c \in Live -> "retired"
                                         [] OTHER -> status[c]]
        /\ dirty' = (dirty \cup surviving) \ (Live \ surviving)
  /\ registrations' = {}
  /\ kernel' = FALSE
  /\ pending' = "none"
  /\ UNCHANGED <<location, source, path>>

TakeChange(c) ==
  /\ c \in dirty
  /\ dirty' = dirty \ {c}
  /\ UNCHANGED <<location, pending, source, status, path, registrations, kernel>>

Stop(to) ==
  /\ source = "available" \/ (source = "unavailable" /\ to = "closed")
  /\ source' = to
  /\ status' = [c \in Cursors |-> IF c \in Live THEN "snapshot" ELSE status[c]]
  /\ registrations' = {}
  /\ kernel' = FALSE
  /\ pending' = "none"
  /\ dirty' = IF to = "unavailable" THEN dirty \cup Live ELSE {}
  /\ UNCHANGED <<location, path>>

Done == source = "closed" /\ UNCHANGED vars

Next ==
  \/ \E c \in Cursors : Open(c) \/ OpenSnapshot(c) \/ Release(c) \/ TakeChange(c)
  \/ \E to \in Paths \cup {Absent} : HostMove(to)
  \/ DispatchMove
  \/ Overflow
  \/ Refresh
  \/ RearmUnavailable
  \/ Stop("unavailable")
  \/ Stop("closed")
  \/ Done

Spec == Init /\ [][Next]_vars /\ WF_vars(DispatchMove \/ Overflow)

LiveHasWatch == Current = {} \/ kernel
NoWatchWithoutCursor == ~kernel \/ (Live # {})
RegistryMatchesSubscribers == registrations = Used(Live)
OneRegistrationPerInode == Cardinality(registrations) <= 1
PendingMoveReconciled == (pending # "none") ~> (pending = "none")
StoppedHasNoWatch == source = "available" \/ (~kernel /\ registrations = {})

=============================================================================
