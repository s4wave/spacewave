---- MODULE Publication ----
\* A handler can finish while a response sink is accepting its offered child.
\* A received handle may be adopted before the sink resumes the producer.
EXTENDS Naturals
CONSTANT FreeWhileSending, PublishBeforeAccept
VARIABLES scope, allocated, native, send, delivered, holder, adopted
vars == <<scope, allocated, native, send, delivered, holder, adopted>>
Init == /\ scope = "open" /\ allocated = FALSE /\ native = FALSE
        /\ send = "idle" /\ delivered = FALSE /\ holder = FALSE /\ adopted = FALSE
Allocate == /\ scope = "open" /\ ~allocated /\ allocated' = TRUE /\ native' = TRUE
            /\ UNCHANGED <<scope, send, delivered, holder, adopted>>
StartSend == /\ scope = "open" /\ native /\ send = "idle" /\ send' = "sending"
             /\ UNCHANGED <<scope, allocated, native, delivered, holder, adopted>>
Receive == /\ send = "sending" /\ ~delivered /\ delivered' = TRUE /\ holder' = TRUE
           /\ UNCHANGED <<scope, allocated, native, send, adopted>>
AcceptSend == /\ send = "sending" /\ delivered /\ send' = "accepted"
              /\ UNCHANGED <<scope, allocated, native, delivered, holder, adopted>>
RejectSend == /\ send = "sending" /\ ~delivered /\ send' = "rejected"
              /\ native' = IF scope = "ended" /\ ~PublishBeforeAccept THEN FALSE ELSE native
              /\ UNCHANGED <<scope, allocated, delivered, holder, adopted>>
Finish == /\ scope = "open" /\ scope' = "ended"
          /\ native' = IF ~adopted /\ send # "accepted" /\ (send # "sending" \/ FreeWhileSending)
                        THEN FALSE ELSE native
          /\ UNCHANGED <<allocated, send, delivered, holder, adopted>>
Adopt == /\ holder /\ native /\ ~adopted /\ adopted' = TRUE
         /\ UNCHANGED <<scope, allocated, native, send, delivered, holder>>
Release == /\ holder /\ holder' = FALSE /\ native' = FALSE
           /\ UNCHANGED <<scope, allocated, send, delivered, adopted>>
Next == Allocate \/ StartSend \/ Receive \/ AcceptSend \/ RejectSend \/ Finish \/ Adopt \/ Release
Spec == Init /\ [][Next]_vars
NoEarlyFree == holder => native
NoRejectedOfferLeak == scope = "ended" /\ send = "rejected" => ~native
=============================================================================
