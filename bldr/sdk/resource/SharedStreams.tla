--------------------------- MODULE SharedStreams ---------------------------
\* SharedStream.subscribe/finally, idle timeout, run completion and close.
\* Two subscribers model SDK Root/Session consumers and navigation rejoin.
\* JS actions run to completion; source I/O interleaves only between actions.
EXTENDS Naturals, FiniteSets
VARIABLES readers, idle, stopped, ended, opened
vars == <<readers, idle, stopped, ended, opened>>
Clients == {1, 2}
Init == /\ readers = {} /\ idle = FALSE /\ stopped = FALSE
        /\ ended = FALSE /\ opened = FALSE
Join(c) == /\ c \notin readers /\ ~stopped /\ ~ended
           /\ readers' = readers \cup {c} /\ idle' = FALSE /\ opened' = TRUE
           /\ UNCHANGED <<stopped, ended>>
Leave(c) == /\ c \in readers /\ readers' = readers \ {c}
            /\ idle' = (readers' = {} /\ ~ended)
            /\ UNCHANGED <<stopped, ended, opened>>
Expire == /\ idle /\ readers = {} /\ idle' = FALSE /\ stopped' = TRUE
          /\ UNCHANGED <<readers, ended, opened>>
End == /\ opened /\ ~ended /\ ended' = TRUE /\ idle' = FALSE
       /\ UNCHANGED <<readers, stopped, opened>>
Next == (\E c \in Clients : Join(c) \/ Leave(c)) \/ Expire \/ End
        \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
LiveReadersKeepSource == stopped => readers = {}
IdleHasNoReaders == idle => readers = {}
=============================================================================
