---------------------------- MODULE MySQLWire ----------------------------
EXTENDS Naturals, FiniteSets, TLC
CONSTANT Clients
VARIABLES state, phase, owner, dirty, published, acknowledged
vars == <<state, phase, owner, dirty, published, acknowledged>>
Init == /\ state = "serving"
        /\ phase = [c \in Clients |-> "idle"]
        /\ owner = "none"
        /\ dirty = FALSE
        /\ published = FALSE
        /\ acknowledged = FALSE
Queue(c) == /\ state = "serving" /\ phase[c] = "idle"
            /\ phase' = [phase EXCEPT ![c] = "waiting"]
            /\ UNCHANGED <<state, owner, dirty, published, acknowledged>>
Acquire(c) == /\ state = "serving" /\ phase[c] = "waiting" /\ owner = "none"
              /\ phase' = [phase EXCEPT ![c] = "working"]
              /\ owner' = c /\ dirty' = FALSE
              /\ UNCHANGED <<state, published, acknowledged>>
Mutate(c) == /\ owner = c /\ phase[c] = "working"
             /\ dirty' = TRUE
             /\ UNCHANGED <<state, phase, owner, published, acknowledged>>
Publish(c) == /\ owner = c /\ phase[c] = "working" /\ dirty
              /\ published' = TRUE
              /\ phase' = [phase EXCEPT ![c] = "published"]
              /\ UNCHANGED <<state, owner, dirty, acknowledged>>
Reply(c) == /\ owner = c /\ phase[c] = "published"
            /\ acknowledged' = TRUE
            /\ owner' = "none" /\ dirty' = FALSE
            /\ phase' = [phase EXCEPT ![c] = "idle"]
            /\ UNCHANGED <<state, published>>
Disconnect(c) == /\ phase[c] # "closed"
                 /\ phase' = [phase EXCEPT ![c] = "closed"]
                 /\ owner' = IF owner = c THEN "none" ELSE owner
                 /\ dirty' = IF owner = c THEN FALSE ELSE dirty
                 /\ UNCHANGED <<state, published, acknowledged>>
Stop == /\ state = "serving" /\ state' = "stopping"
        /\ UNCHANGED <<phase, owner, dirty, published, acknowledged>>
Stopped == /\ state = "stopping"
           /\ \A c \in Clients: phase[c] = "closed"
           /\ state' = "stopped"
           /\ UNCHANGED <<phase, owner, dirty, published, acknowledged>>
Done == /\ state = "stopped" /\ UNCHANGED vars
Drain(c) == /\ state = "stopping" /\ Disconnect(c)
Next == (\E c \in Clients: Queue(c) \/ Acquire(c) \/ Mutate(c) \/ Publish(c) \/ Reply(c) \/ Disconnect(c))
        \/ Stop \/ Stopped \/ Done
Spec == Init /\ [][Next]_vars /\ (\A c \in Clients: WF_vars(Drain(c))) /\ WF_vars(Stopped)
AckIsPublished == acknowledged => published
ClosedOwnsNothing == \A c \in Clients: phase[c] = "closed" => owner # c
StopReleases == state = "stopped" => owner = "none"
StopCompletes == state = "stopping" ~> state = "stopped"
ClosedTerminal == \A c \in Clients: phase[c] = "closed" => phase'[c] = "closed"
NoResurrection == [][ClosedTerminal]_vars
=============================================================================
