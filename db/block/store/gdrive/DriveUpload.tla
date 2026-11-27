--------------------------- MODULE DriveUpload ---------------------------
EXTENDS Naturals, TLC
\* One immutable upload: chunks remain hidden until the final successful PUT.
\* An expired session loses only unpublished chunks. Cancellation/retry may
\* leave duplicate complete files; neither can delete an existing file.
CONSTANT Chunks
VARIABLES committed, visible, session
vars == <<committed, visible, session>>
Init == /\ committed = 0 /\ visible = FALSE /\ session = FALSE
Start == /\ ~session /\ session' = TRUE /\ committed' = 0 /\ UNCHANGED visible
Chunk == /\ session /\ committed < Chunks
         /\ committed' = committed + 1
         /\ visible' = (visible \/ committed' = Chunks)
         /\ UNCHANGED session
Expire == /\ session /\ ~visible
          /\ session' = FALSE /\ committed' = 0 /\ UNCHANGED visible
Done == /\ visible /\ committed = Chunks /\ UNCHANGED vars
Next == Start \/ Chunk \/ Expire \/ Done
TypeOK == /\ committed \in 0..Chunks /\ visible \in BOOLEAN /\ session \in BOOLEAN
CompleteBeforeVisible == visible => committed = Chunks
Spec == Init /\ [][Next]_vars
=============================================================================
