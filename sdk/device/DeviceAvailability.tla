---------------------- MODULE DeviceAvailability ----------------------
EXTENDS Naturals, TLC
CONSTANT MaxChanges
VARIABLES device, observable, transport, link, sourceRev, sampledRev, availability, waiting
vars == <<device, observable, transport, link, sourceRev, sampledRev, availability, waiting>>

(* Device state and transport state are sampled with their notification channel.
   A link event invalidates the sample without writing the durable Device.
   The UI renders Loading until the first sample, then Unknown when either
   identity is unavailable, or Online or Offline from authenticated links.
   Stored liveness is deliberately absent from the reachability calculation. *)
Init == /\ device = FALSE /\ observable = FALSE /\ transport = FALSE /\ link = FALSE
        /\ sourceRev = 0 /\ sampledRev = 0
        /\ availability = "loading" /\ waiting = FALSE

Change(d, o, t, l) ==
  /\ sourceRev < MaxChanges
  /\ device' = d /\ observable' = o /\ transport' = t /\ link' = l
  /\ sourceRev' = sourceRev + 1 /\ waiting' = FALSE
  /\ UNCHANGED <<sampledRev, availability>>

Observe ==
  /\ ~waiting
  /\ availability' = IF ~device THEN "loading"
                      ELSE IF ~observable THEN "unknown"
                      ELSE IF transport /\ link THEN "online" ELSE "offline"
  /\ sampledRev' = sourceRev /\ waiting' = TRUE
  /\ UNCHANGED <<device, observable, transport, link, sourceRev>>

\* Done ends a run: the model's changes are spent and the last one is observed.
Done == sourceRev = MaxChanges /\ waiting /\ UNCHANGED vars
Next == \/ Change(TRUE, observable, transport, link)
        \/ Change(device, ~observable, transport, link)
        \/ Change(device, observable, TRUE, TRUE)
        \/ Change(device, observable, transport, FALSE)
        \/ Change(device, observable, FALSE, FALSE)
        \/ Observe
        \/ Done
Spec == Init /\ [][Next]_vars /\ WF_vars(Observe)
NoMissedWake == waiting => sampledRev = sourceRev
TruthfulSample == sampledRev = sourceRev /\ waiting =>
  availability = IF ~device THEN "loading"
                 ELSE IF ~observable THEN "unknown"
                 ELSE IF transport /\ link THEN "online" ELSE "offline"
Settles == <>[](waiting /\ sampledRev = sourceRev)
=============================================================================
