---------------------------- MODULE ObjectSink ----------------------------
EXTENDS Naturals, FiniteSets
CONSTANTS Acts, Vals, MaxVisits, None
VARIABLES task, out, port, done, obj, written, paused
vars == <<task, out, port, done, obj, written, paused>>

(* Models runner.route delivering a Step's chosen output to an object sink:
   a node bound to an object key, with the World type its object must have.
   Routing marks the activation done and commits the output as the object's
   state in one World transaction, so no write happens without its routing
   and none twice. A sink whose object another writer gave another type
   refuses the write, keeps the activation's inputs and pauses the run, as a
   failed Step does; a resume runs the Step again as its next visit. Another
   writer may retype or delete the object at any time. An activation is
   identified by its Step input and its visit; written records each commit
   as the activation, its value and the type of the object it replaced. *)
Ids == Acts \X (1..MaxVisits)
Types == {None, "sink", "other"}
Init == /\ task = [id \in Ids |-> IF id[2] = 1 THEN "running" ELSE "idle"]
        /\ out = [id \in Ids |-> None]
        /\ port = [id \in Ids |-> None]
        /\ done = [id \in Ids |-> FALSE]
        /\ obj = [type |-> None, val |-> None]
        /\ written = {}
        /\ paused = FALSE

\* The Task ends: it succeeds with a value on one port, or fails with none.
Succeed(id, v, p) == /\ task[id] = "running"
                     /\ task' = [task EXCEPT ![id] = "ok"]
                     /\ out' = [out EXCEPT ![id] = v]
                     /\ port' = [port EXCEPT ![id] = p]
                     /\ UNCHANGED <<done, obj, written, paused>>
Fail(id) == /\ task[id] = "running"
            /\ task' = [task EXCEPT ![id] = "failed"]
            /\ UNCHANGED <<out, port, done, obj, written, paused>>

\* Requeue keeps the inputs and pauses the run at the Step.
Requeue(id) == /\ done' = [done EXCEPT ![id] = TRUE]
               /\ paused' = TRUE
               /\ UNCHANGED <<task, out, port, obj, written>>

\* One advance transaction routes one completed activation.
Route(id) == /\ task[id] \in {"ok", "failed"}
             /\ ~done[id]
             /\ IF task[id] = "failed" THEN Requeue(id)
                ELSE IF port[id] # "sink"
                  THEN /\ done' = [done EXCEPT ![id] = TRUE]
                       /\ UNCHANGED <<task, out, port, obj, written, paused>>
                ELSE IF obj.type \in {None, "sink"}
                  THEN /\ done' = [done EXCEPT ![id] = TRUE]
                       /\ obj' = [type |-> "sink", val |-> out[id]]
                       /\ written' = written \cup {<<id, out[id], obj.type>>}
                       /\ UNCHANGED <<task, out, port, paused>>
                ELSE Requeue(id)

\* A resume runs a paused Step's requeued inputs again as its next visit.
Resume(a, n) == /\ paused
                /\ n < MaxVisits
                /\ done[<<a, n>>]
                /\ task[<<a, n>>] = "failed" \/ port[<<a, n>>] = "sink"
                /\ ~\E w \in written : w[1] = <<a, n>>
                /\ task[<<a, n + 1>>] = "idle"
                /\ task' = [task EXCEPT ![<<a, n + 1>>] = "running"]
                /\ paused' = FALSE
                /\ UNCHANGED <<out, port, done, obj, written>>

\* Another writer retypes or deletes the object.
Retype == /\ obj' = [type |-> "other", val |-> "theirs"]
          /\ UNCHANGED <<task, out, port, done, written, paused>>
Delete == /\ obj' = [type |-> None, val |-> None]
          /\ UNCHANGED <<task, out, port, done, written, paused>>

Next == \/ \E id \in Ids : \/ \E v \in Vals, p \in {"sink", "other"} : Succeed(id, v, p)
                           \/ Fail(id)
                           \/ Route(id)
        \/ \E a \in Acts, n \in 1..MaxVisits : Resume(a, n)
        \/ Retype
        \/ Delete
Spec == Init /\ [][Next]_vars

\* The sink's object holds only a value a routed activation chose for it.
NoInvented == obj.type = "sink" => \E w \in written : w[2] = obj.val
\* Each write commits with its routing, from a success on the sink's port.
WriteWithRouting == \A w \in written : /\ done[w[1]]
                                       /\ task[w[1]] = "ok"
                                       /\ port[w[1]] = "sink"
                                       /\ out[w[1]] = w[2]
\* An activation writes at most once.
WriteOnce == \A id \in Ids : Cardinality({w \in written : w[1] = id}) <= 1
\* The sink never overwrites an object another writer typed otherwise: each
\* write replaced an absent object or one of the sink's type.
NoWrongTarget == \A w \in written : w[3] \in {None, "sink"}
\* A success the sink could not commit pauses the run instead of vanishing.
RefusalPauses == \A id \in Ids :
  (done[id] /\ task[id] = "ok" /\ port[id] = "sink" /\ ~\E w \in written : w[1] = id)
    => (paused \/ (id[2] < MaxVisits /\ task[<<id[1], id[2] + 1>>] # "idle"))
=============================================================================
