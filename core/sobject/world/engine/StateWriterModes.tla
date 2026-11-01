---- MODULE StateWriterModes ----
\* Controls select the store's effective lane. Unavailable preparation on a
\* local placement must not refuse an otherwise valid control operation.
EXTENDS Naturals
CONSTANTS Lane, GateControls
VARIABLE pc
Init == pc = "control"
Choose == /\ pc = "control" /\ pc' = IF Lane \/ ~GateControls THEN "prepare" ELSE "engine"
Prepare == /\ pc = "prepare" /\ pc' = IF Lane THEN "engine" ELSE "refused"
Engine == /\ pc = "engine" /\ pc' = "done"
Next == Choose \/ Prepare \/ Engine \/ UNCHANGED pc
Spec == Init /\ [][Next]_pc
Available == pc # "refused"
====
