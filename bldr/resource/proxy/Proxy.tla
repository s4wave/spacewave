------------------------------ MODULE Proxy ------------------------------
\* ResourceClient generations, ResourceRpc retries and child rebinding.
\* Callers: a root client and one child; two upstreams and one unary call.
\* Requests and responses interleave with lose/bind. Packet encoding,
\* transport fairness and nested child depth are checked by the Go tests.
EXTENDS Naturals
VARIABLES upstream, bound, child, released, sent, attempt, pending, answered,
          delivered, version, held, replayed
vars == <<upstream, bound, child, released, sent, attempt, pending, answered,
          delivered, version, held, replayed>>
Init == /\ upstream = 1 /\ bound = 0 /\ child = 0 /\ released = FALSE
        /\ sent = FALSE /\ attempt = 0 /\ pending = FALSE
        /\ answered = FALSE /\ delivered = 0 /\ version = 0 /\ held = 0
        /\ replayed = 0
Bind == /\ bound # upstream /\ replayed = upstream
        /\ bound' = upstream
        /\ UNCHANGED <<upstream, child, released, sent, attempt, pending,
                       answered, delivered, version, held, replayed>>
Replay == /\ replayed # upstream /\ replayed' = upstream
          /\ UNCHANGED <<upstream, bound, child, released, sent, attempt,
                         pending, answered, delivered, version, held>>
OpenChild == /\ bound = upstream /\ ~released /\ child # upstream
             /\ child' = upstream
             /\ UNCHANGED <<upstream, bound, released, sent, attempt, pending,
                            answered, delivered, version, held, replayed>>
SendOnce == /\ bound = upstream /\ child = upstream /\ ~released
            /\ ~sent /\ ~answered /\ sent' = TRUE
            /\ attempt' = upstream /\ pending' = TRUE
            /\ UNCHANGED <<upstream, bound, child, released, answered,
                           delivered, version, held, replayed>>
Answer == /\ pending /\ attempt = upstream /\ bound = upstream
          /\ ~released /\ pending' = FALSE /\ answered' = TRUE
          /\ delivered' = delivered + 1
          /\ UNCHANGED <<upstream, bound, child, released, sent, attempt,
                         version, held, replayed>>
Lose == /\ upstream = 1 /\ upstream' = 2
        /\ pending' = FALSE
        /\ UNCHANGED <<bound, child, released, sent, attempt, answered,
                       delivered, version, held, replayed>>
Publish == /\ version < 2 /\ version' = version + 1
           /\ UNCHANGED <<upstream, bound, child, released, sent, attempt,
                          pending, answered, delivered, held, replayed>>
Resume == /\ bound = upstream /\ ~released /\ held' = version
          /\ UNCHANGED <<upstream, bound, child, released, sent, attempt,
                         pending, answered, delivered, version, replayed>>
Release == /\ ~released /\ released' = TRUE /\ pending' = FALSE
           /\ UNCHANGED <<upstream, bound, child, sent, attempt, answered,
                          delivered, version, held, replayed>>
Next == Bind \/ Replay \/ OpenChild \/ SendOnce \/ Answer \/ Lose \/ Publish
        \/ Resume \/ Release \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
OnceAnswered == delivered <= 1
HeldVersion == held <= version
ReplyGeneration == answered => attempt <= upstream
=============================================================================
