---------------------------- MODULE ProxyCompletion ----------------------------
VARIABLES packet, sent, read, phase, result
vars == <<packet, sent, read, phase, result>>
Terminal == {"success", "method error"}

(* ProxyInvoker forwards a request and reads the response in separate
   goroutines. It can select either result first. Closing the stream after a
   failed write wakes the reader and leaves any queued terminal packet
   readable. The model abstracts the body packets and transport framing. *)
Init == /\ packet = "none" /\ sent = "pending" /\ read = "pending"
        /\ phase = "selecting" /\ result = "none"

QueueTerminal(outcome) == /\ packet = "none" /\ read = "pending"
                          /\ phase # "done" /\ (phase # "draining" \/ sent = "sent")
                          /\ packet' = outcome
                          /\ UNCHANGED <<sent, read, phase, result>>

FinishWrite(outcome) == /\ sent = "pending" /\ phase = "selecting"
                        /\ sent' = outcome
                        /\ UNCHANGED <<packet, read, phase, result>>

ReadResponse == /\ read = "pending"
                /\ packet \in Terminal \/ (phase = "draining" /\ sent = "failed")
                /\ read' = IF packet \in Terminal THEN packet ELSE "read error"
                /\ UNCHANGED <<packet, sent, phase, result>>

SelectRead == /\ phase = "selecting" /\ read # "pending"
              /\ phase' = "done" /\ result' = read
              /\ UNCHANGED <<packet, sent, read>>

SelectWrite == /\ phase = "selecting" /\ sent # "pending"
               /\ phase' = "draining"
               /\ UNCHANGED <<packet, sent, read, result>>

ReturnAfterWrite == /\ phase = "draining" /\ read # "pending"
                    /\ phase' = "done"
                    /\ result' = IF sent = "failed" /\ read \notin Terminal
                                 THEN "write error" ELSE read
                    /\ UNCHANGED <<packet, sent, read>>

Done == phase = "done" /\ UNCHANGED vars
Next == \/ \E outcome \in Terminal : QueueTerminal(outcome)
        \/ \E outcome \in {"sent", "failed"} : FinishWrite(outcome)
        \/ ReadResponse \/ SelectRead \/ SelectWrite \/ ReturnAfterWrite \/ Done
Spec == Init /\ [][Next]_vars /\ WF_vars(ReadResponse)
        /\ WF_vars(SelectWrite) /\ WF_vars(ReturnAfterWrite)

KeepsRemoteOutcome == (phase = "done" /\ read \in Terminal) => result = read
FailedWriteReturns == (sent = "failed") ~> (phase = "done")
=============================================================================
