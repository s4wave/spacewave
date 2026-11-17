-------------------------- MODULE RelayLifetime --------------------------
\* GenerateCode and CompletePeer both runRelay. Clear/begin cancel the attempt;
\* SessionTransport.Execute owns the child bus independently of that attempt.
\* WithChildBus captures its bus/context under bcast, then installs AfterFunc.
\* Callback delivery and cancellation observation are separate scheduled steps.
\* One generation suffices: a worker retains its captured bus through replacement.
\* Link establishment and solicitation are abstract waits on the scoped context.
\* Enrollment, HTTP registration, direct streams and bus internals are omitted.
EXTENDS Naturals
VARIABLES transport, attempt, phase, captured, bound, canceled, nilBus
vars == <<transport, attempt, phase, captured, bound, canceled, nilBus>>
Init == /\ transport = TRUE /\ attempt = TRUE /\ phase = "acquire"
        /\ captured = FALSE /\ bound = FALSE /\ canceled = FALSE
        /\ nilBus = FALSE
StopTransport == /\ transport /\ transport' = FALSE
                 /\ UNCHANGED <<attempt, phase, captured, bound, canceled, nilBus>>
ClearAttempt == /\ attempt /\ attempt' = FALSE
                /\ UNCHANGED <<transport, phase, captured, bound, canceled, nilBus>>
Acquire == /\ phase = "acquire"
           /\ captured' = transport
           /\ phase' = IF transport THEN "bind" ELSE "done"
           /\ UNCHANGED <<transport, attempt, bound, canceled, nilBus>>
Bind == /\ phase = "bind" /\ bound' = TRUE /\ phase' = "check"
        /\ UNCHANGED <<transport, attempt, captured, canceled, nilBus>>
Cancel == /\ bound /\ ~canceled /\ (~transport \/ ~attempt)
          /\ canceled' = TRUE
          /\ UNCHANGED <<transport, attempt, phase, captured, bound, nilBus>>
Enter == /\ phase = "check"
         /\ phase' = IF ~transport \/ canceled THEN "done" ELSE "work"
         /\ UNCHANGED <<transport, attempt, captured, bound, canceled, nilBus>>
UseBus == /\ phase = "work"
          /\ nilBus' = ~captured
          /\ phase' = "wait"
          /\ UNCHANGED <<transport, attempt, captured, bound, canceled>>
Exit == /\ phase = "wait" /\ canceled /\ phase' = "done"
        /\ UNCHANGED <<transport, attempt, captured, bound, canceled, nilBus>>
Worker == Acquire \/ Bind \/ Enter \/ UseBus \/ Exit
Done == /\ phase = "done" /\ UNCHANGED vars
Next == StopTransport \/ ClearAttempt \/ Cancel \/ Worker \/ Done
Spec == Init /\ [][Next]_vars /\ WF_vars(Worker) /\ WF_vars(Cancel)
BusRetained == ~nilBus
OwnerEndsWorker == (~transport \/ ~attempt) ~> (phase = "done")
=============================================================================
