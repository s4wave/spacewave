---- MODULE ReplayRefusalHandoff ----
\* Adapter.Apply finds no candidate for a pre-refused order entry: the entry
\* carries no decoded body. After replay computes the refusal, noteUnmatched
\* and the opQueue's queued callback exchange evidence under Adapter.bcast.
\* The fork has the same signed local ID and nonce, but a different hash.
\* Payload adoption and checkpoint inference belong to other replay paths.

VARIABLES ack, exact, fork, seen, decided
vars == <<ack, exact, fork, seen, decided>>
Init == /\ ack = FALSE /\ exact = "unseen" /\ fork = "unseen"
        /\ seen = {} /\ decided = "pending"

\* Apply's first lookup can miss before queued publishes the identity.
Match == /\ exact = "unseen"
         /\ exact' = IF ack THEN "matched" ELSE "missed"
         /\ UNCHANGED <<ack, fork, seen, decided>>

\* The replay side must meet the callback here, not only at Match.
NoteExact == /\ exact \in {"matched", "missed"}
             /\ exact' = "noted"
             /\ decided' = IF ack THEN "exact" ELSE decided
             /\ seen' = IF ack THEN seen ELSE seen \cup {"exact"}
             /\ UNCHANGED <<ack, fork>>
NoteFork == /\ fork = "unseen"
            /\ fork' = "noted" /\ seen' = seen \cup {"fork"}
            /\ UNCHANGED <<ack, exact, decided>>
Queued == /\ ~ack
          /\ ack' = TRUE
          /\ decided' = IF "exact" \in seen THEN "exact" ELSE decided
          /\ seen' = seen \ {"exact"}
          /\ UNCHANGED <<exact, fork>>
Done == /\ ack /\ exact = "noted" /\ fork = "noted"
        /\ UNCHANGED vars
Next == Match \/ NoteExact \/ NoteFork \/ Queued \/ Done
NoFalseRefusal == decided # "pending" => decided = "exact" /\ exact = "noted"
BothOrdersDecide == ack /\ exact = "noted" => decided = "exact"

\* Exact replay: one pending World admission and two distinct positions on
\* the local chain. Older is already decided but may be replayed in a reorder
\* with the same bytes as the pending one. A pre-refusal may have no body.
\* The local and Spacewave providers both call SOHost.AddLocalOperation.
\* Sign publishes the receipt before the state can expose the new position.
\* QueuedExact is the successful QueueOperation callback, not acceptance.
\* ReplayExact abstracts the driver Apply call; InstallExact is its head
\* install (possibly delayed by missing positions). Checkpoint chain proof
\* and store failures are outside this attribution model. ExpireExact is the
\* clock/caller ending its wait after dequeue; it does not remove the admission.
VARIABLES signed, published, returned, older, current, placed, installed, answer, expired, owned
exactVars == <<signed, published, returned, older, current, placed, installed, answer, expired, owned>>
ExactInit == /\ signed = "none" /\ published = FALSE /\ returned = FALSE
             /\ older = "unseen" /\ current = "unseen"
             /\ placed = "none" /\ installed = "none" /\ answer = "pending"
             /\ expired = FALSE /\ owned = TRUE

Sign == /\ signed = "none"
        /\ signed' = "current" /\ published' = TRUE
        /\ UNCHANGED <<returned, older, current, placed, installed, answer, expired, owned>>

QueuedExact == /\ signed # "none" /\ ~returned
               /\ returned' = TRUE
               /\ UNCHANGED <<signed, published, older, current, placed, installed, answer, expired, owned>>

Matches(hash, outcome) == published /\ signed = hash

ReplayOlder(outcome) == /\ older = "unseen"
                        /\ older' = outcome
                        /\ placed' = IF owned /\ Matches("older", outcome) /\ outcome = "applied"
                                     THEN "older" ELSE placed
                        /\ answer' = IF owned /\ Matches("older", outcome) /\ outcome = "refused"
                                     THEN "rejected-older" ELSE answer
                        /\ owned' = IF answer' = "rejected-older" THEN FALSE ELSE owned
                        /\ UNCHANGED <<signed, published, returned, current, installed, expired>>

ReplayExact(outcome) == /\ signed # "none" /\ current = "unseen"
                        /\ current' = outcome
                        /\ placed' = IF owned /\ Matches("current", outcome) /\ outcome = "applied"
                                     THEN "current" ELSE placed
                        /\ answer' = IF owned /\ Matches("current", outcome) /\ outcome = "refused"
                                     THEN "rejected-current" ELSE answer
                        /\ owned' = IF answer' = "rejected-current" THEN FALSE ELSE owned
                        /\ UNCHANGED <<signed, published, returned, older, installed, expired>>

InstallExact == /\ installed = "none" /\ placed # "none"
                /\ installed' = placed /\ answer' = "accepted" /\ owned' = FALSE
                /\ UNCHANGED <<signed, published, returned, older, current, placed, expired>>

ExpireExact == /\ ~expired /\ answer = "pending"
               /\ expired' = TRUE /\ owned' = owned
               /\ UNCHANGED <<signed, published, returned, older, current, placed, installed, answer>>

ExactDone == /\ returned /\ answer # "pending" /\ older # "unseen"
             /\ UNCHANGED exactVars
ExactNext == Sign \/ QueuedExact \/ InstallExact \/ ExpireExact \/ ExactDone
             \/ \E outcome \in {"applied", "refused"}: ReplayOlder(outcome) \/ ReplayExact(outcome)
AllVars == <<vars, exactVars>>
BothInit == Init /\ ExactInit
LegacyNext == Next /\ UNCHANGED exactVars
ReceiptNext == ExactNext /\ UNCHANGED vars
Spec == BothInit /\ [][LegacyNext]_AllVars
ExactSpec == BothInit /\ [][ReceiptNext]_AllVars
ExactAcceptance == answer = "accepted" => installed = "current" /\ current = "applied"
ExactRefusal == answer \in {"rejected-older", "rejected-current"} =>
                answer = "rejected-current" /\ current = "refused"
ReplayOwnsPending == answer = "pending" => owned
RefusalDecides == current = "refused" => answer = "rejected-current"
AppliedPlacesExact == current = "applied" => placed = "current"
====
