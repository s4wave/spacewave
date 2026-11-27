------------------------- MODULE PacketDeadlineSnapshots -------------------------
EXTENDS Naturals, Sequences, TLC

\* Models PacketConn in websocket and packetConn in inproc. QUIC Transport.Close,
\* its maybeStopListening timer and application deadline setters can overlap.
\* Packet readers and writers observe a time.Time represented by two words.
\* Network I/O and connection cancellation do not access the deadline words and
\* are outside this publication model. No lock is held during I/O.
\* FALSE models the old shared struct assignment; TRUE prepares an immutable
\* private time.Time and publishes its pointer in one atomic store.
ImmutableSnapshots == TRUE
Writers == {1, 2}
VARIABLES phase, local, published, observed
vars == <<phase, local, published, observed>>

Init == /\ phase = [w \in Writers |-> "ready"]
        /\ local = [w \in Writers |-> <<0, 0>>]
        /\ published = <<0, 0>>
        /\ observed = <<0, 0>>

Prepare(w) == /\ phase[w] = "ready"
              /\ phase' = [phase EXCEPT ![w] = "prepared"]
              /\ IF ImmutableSnapshots
                    THEN /\ local' = [local EXCEPT ![w] = <<w, 0>>]
                         /\ UNCHANGED published
                    ELSE /\ published' = <<w, published[2]>>
                         /\ UNCHANGED local
              /\ UNCHANGED observed

Publish(w) == /\ phase[w] = "prepared"
              /\ phase' = [phase EXCEPT ![w] = "done"]
              /\ IF ImmutableSnapshots
                    THEN /\ local' = [local EXCEPT ![w] = <<w, w>>]
                         /\ published' = <<local[w][1], w>>
                    ELSE /\ published' = <<published[1], w>>
                         /\ UNCHANGED local
              /\ UNCHANGED observed

Observe == /\ observed' = published
           /\ UNCHANGED <<phase, local, published>>
Done == /\ \A w \in Writers : phase[w] = "done"
        /\ UNCHANGED vars
Next == (\E w \in Writers : Prepare(w) \/ Publish(w)) \/ Observe \/ Done
Spec == Init /\ [][Next]_vars
NoTornTime == observed[1] = observed[2]
===============================================================================
