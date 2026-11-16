-------------------------- MODULE EvictionPassLog --------------------------
\* runEviction is the sole producer. Each completed pass sends four counts;
\* an empty result is ignored. Its next pass may block indefinitely in a gate,
\* admission or backend call. The logger alone owns the totals and timer.
\* A timer expires independently of that pass. Closing the channel on success,
\* cancellation or error flushes the remainder; the producer joins the worker.
\* Three passes and one abstract minute per active interval bound this model.
\* Vectors abstract the four independent additive counters. Store operations,
\* logrus internals and scheduler latency are outside this logging lifecycle.
EXTENDS Naturals, Sequences
VARIABLES producer, worker, inPass, sent, pending, accepted, logged, armed, due, lines
vars == <<producer, worker, inPass, sent, pending, accepted, logged, armed, due, lines>>
Zero == <<0, 0, 0, 0>>
Results == {Zero, <<1, 2, 3, 4>>, <<2, 1, 0, 1>>}
Add(a, b) == [i \in 1..4 |-> a[i] + b[i]]
Init == /\ producer = "running" /\ worker = "receiving" /\ inPass = FALSE
        /\ sent = 0 /\ pending = Zero /\ accepted = Zero /\ logged = Zero
        /\ armed = FALSE /\ due = FALSE /\ lines = <<>>

StartPass == /\ producer = "running" /\ ~inPass /\ sent < 3
             /\ inPass' = TRUE
             /\ UNCHANGED <<producer, worker, sent, pending, accepted, logged, armed, due, lines>>

Receive == /\ producer = "running" /\ worker = "receiving" /\ inPass
           /\ \E result \in Results :
                /\ pending' = Add(IF due THEN Zero ELSE pending, result)
                /\ accepted' = Add(accepted, result)
                /\ logged' = IF due THEN Add(logged, pending) ELSE logged
                /\ lines' = IF due THEN Append(lines, pending) ELSE lines
                /\ armed' = (pending' # Zero) /\ due' = FALSE
           /\ sent' = sent + 1 /\ inPass' = FALSE
           /\ UNCHANGED <<producer, worker>>

Expire == /\ worker = "receiving" /\ armed /\ ~due
          /\ due' = TRUE
          /\ UNCHANGED <<producer, worker, inPass, sent, pending, accepted, logged, armed, lines>>

FlushDue == /\ worker = "receiving" /\ due
            /\ logged' = Add(logged, pending) /\ lines' = Append(lines, pending)
            /\ pending' = Zero /\ armed' = FALSE /\ due' = FALSE
            /\ UNCHANGED <<producer, worker, inPass, sent, accepted>>

Close == /\ producer = "running"
         /\ producer' = "closing" /\ inPass' = FALSE
         /\ UNCHANGED <<worker, sent, pending, accepted, logged, armed, due, lines>>

StopWorker == /\ producer = "closing" /\ worker = "receiving"
              /\ logged' = Add(logged, pending)
              /\ lines' = IF pending = Zero THEN lines ELSE Append(lines, pending)
              /\ pending' = Zero /\ armed' = FALSE /\ due' = FALSE /\ worker' = "stopped"
              /\ UNCHANGED <<producer, inPass, sent, accepted>>

Join == /\ producer = "closing" /\ worker = "stopped"
        /\ producer' = "stopped"
        /\ UNCHANGED <<worker, inPass, sent, pending, accepted, logged, armed, due, lines>>

Stopped == /\ producer = "stopped" /\ UNCHANGED vars
Next == StartPass \/ Receive \/ Expire \/ FlushDue \/ Close \/ StopWorker \/ Join \/ Stopped
Spec == Init /\ [][Next]_vars /\ WF_vars(FlushDue) /\ WF_vars(StopWorker) /\ WF_vars(Join)
CountsPreserved == Add(logged, pending) = accepted
NoEmptyLines == \A i \in 1..Len(lines) : lines[i] # Zero
Joined == producer = "stopped" => worker = "stopped" /\ ~armed /\ pending = Zero
DeadlineFlushes == due ~> ~due
StopCompletes == producer = "closing" ~> producer = "stopped"
=============================================================================
