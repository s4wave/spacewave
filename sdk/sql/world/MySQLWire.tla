---------------------------- MODULE MySQLWire ----------------------------
EXTENDS Naturals, FiniteSets, TLC
CONSTANT Clients
VARIABLES state, phase, owner, dirty, private, published, acknowledged
vars == <<state, phase, owner, dirty, private, published, acknowledged>>
Init == /\ state = "serving"
        /\ phase = [c \in Clients |-> "idle"]
        /\ owner = "none"
        /\ dirty = FALSE
        /\ private = FALSE
        /\ published = FALSE
        /\ acknowledged = FALSE
Queue(c) == /\ state = "serving" /\ phase[c] = "idle"
            /\ phase' = [phase EXCEPT ![c] = "waiting"]
            /\ UNCHANGED <<state, owner, dirty, private, published, acknowledged>>
Acquire(c) == /\ state = "serving" /\ phase[c] = "waiting" /\ owner = "none"
              /\ phase' = [phase EXCEPT ![c] = "working"]
              /\ owner' = c /\ dirty' = FALSE
              /\ UNCHANGED <<state, private, published, acknowledged>>
Mutate(c) == /\ owner = c /\ phase[c] = "working"
             /\ dirty' = TRUE
             /\ UNCHANGED <<state, phase, owner, private, published, acknowledged>>
\* Commit persists the SQL root, which stays private until the World holds it.
Commit(c) == /\ owner = c /\ phase[c] = "working" /\ dirty
             /\ private' = TRUE
             /\ phase' = [phase EXCEPT ![c] = "committing"]
             /\ UNCHANGED <<state, owner, dirty, published, acknowledged>>
Publish(c) == /\ owner = c /\ phase[c] = "committing"
              /\ private' = FALSE /\ published' = TRUE
              /\ phase' = [phase EXCEPT ![c] = "published"]
              /\ UNCHANGED <<state, owner, dirty, acknowledged>>
Reply(c) == /\ owner = c /\ phase[c] = "published"
            /\ acknowledged' = TRUE
            /\ owner' = "none" /\ dirty' = FALSE
            /\ phase' = [phase EXCEPT ![c] = "idle"]
            /\ UNCHANGED <<state, private, published>>
\* A client that leaves cannot interrupt its commit: the socket closes at once,
\* and the session closes after publication.
Disconnect(c) == /\ phase[c] \notin {"closed", "committing"}
                 /\ phase' = [phase EXCEPT ![c] = "closed"]
                 /\ owner' = IF owner = c THEN "none" ELSE owner
                 /\ dirty' = IF owner = c THEN FALSE ELSE dirty
                 /\ UNCHANGED <<state, private, published, acknowledged>>
\* Only the endpoint's stop interrupts a commit. Its root stays private, and
\* a stopping endpoint admits no client to see it.
Abort(c) == /\ state = "stopping" /\ owner = c /\ phase[c] = "committing"
            /\ phase' = [phase EXCEPT ![c] = "closed"]
            /\ owner' = "none" /\ dirty' = FALSE
            /\ UNCHANGED <<state, private, published, acknowledged>>
Stop == /\ state = "serving" /\ state' = "stopping"
        /\ UNCHANGED <<phase, owner, dirty, private, published, acknowledged>>
Stopped == /\ state = "stopping"
           /\ \A c \in Clients: phase[c] = "closed"
           /\ state' = "stopped"
           /\ UNCHANGED <<phase, owner, dirty, private, published, acknowledged>>
Done == /\ state = "stopped" /\ UNCHANGED vars
Drain(c) == /\ state = "stopping" /\ (Disconnect(c) \/ Abort(c))
Next == (\E c \in Clients: Queue(c) \/ Acquire(c) \/ Mutate(c) \/ Commit(c) \/ Publish(c) \/ Reply(c)
                            \/ Disconnect(c) \/ Abort(c))
        \/ Stop \/ Stopped \/ Done
Spec == Init /\ [][Next]_vars /\ (\A c \in Clients: WF_vars(Drain(c))) /\ WF_vars(Stopped)
AckIsPublished == acknowledged => published
PrivateIsOwned == (private /\ state = "serving") => \E c \in Clients: owner = c /\ phase[c] = "committing"
ClosedOwnsNothing == \A c \in Clients: phase[c] = "closed" => owner # c
StopReleases == state = "stopped" => owner = "none"
StopCompletes == state = "stopping" ~> state = "stopped"
ClosedTerminal == \A c \in Clients: phase[c] = "closed" => phase'[c] = "closed"
NoResurrection == [][ClosedTerminal]_vars
=============================================================================
