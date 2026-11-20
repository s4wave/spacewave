---- MODULE UploadStage ----
\* One name in the portable staging namespace. The kernel grants one
\* exclusive lease at a time. Unmarked names can come from host files
\* or an interrupted lease allocation. Anonymous inodes have no name
\* for the collector to visit. Allocate abstracts a successful
\* exclusive create and its marker, before any upload bytes are written.
VARIABLES marker, held, payload, published, hostOwns
vars == <<marker, held, payload, published, hostOwns>>

Init ==
  /\ marker = FALSE
  /\ held = FALSE
  /\ hostOwns \in BOOLEAN
  /\ payload = IF hostOwns THEN "unowned" ELSE "absent"
  /\ published = FALSE

Allocate ==
  /\ payload = "absent" /\ ~marker
  /\ marker' = TRUE /\ held' = TRUE
  /\ UNCHANGED <<payload, published, hostOwns>>

Stage ==
  /\ marker /\ held /\ payload = "absent"
  /\ payload' = "staged"
  /\ UNCHANGED <<marker, held, published, hostOwns>>

Publish ==
  /\ held /\ payload = "staged" /\ ~published
  /\ published' = TRUE /\ payload' = "absent"
  /\ UNCHANGED <<marker, held, hostOwns>>

End == held /\ held' = FALSE /\ UNCHANGED <<marker, payload, published, hostOwns>>

Sweep ==
  /\ marker /\ ~held
  /\ marker' = FALSE /\ payload' = "absent"
  /\ UNCHANGED <<held, published, hostOwns>>

Next == Allocate \/ Stage \/ Publish \/ End \/ Sweep
Spec == Init /\ [][Next]_vars

UnownedPreserved == hostOwns => payload = "unowned"
StageHasMarker == payload = "staged" => marker
LeaseHasMarker == held => marker
====
