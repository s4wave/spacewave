------------------------- MODULE ActivationActors -------------------------
EXTENDS Naturals, FiniteSets
CONSTANTS Keys, MaxVisits
VARIABLES active, receipt, objects, attached, visits, history
vars == <<active, receipt, objects, attached, visits, history>>

(* Models keyed activation creation by runner.activate, the test template
   runner's atomic actor creation + RecordActivationActor, exec reattachment,
   and runner.route. GLaDOS and external actor side effects are outside this
   package. Restart clears only the exec attachment; the World survives.
   Task identity includes the visit, so a successor gets a fresh actor. *)
Ids == Keys \X (1..MaxVisits)
Init == /\ active = {}
        /\ receipt = [id \in Ids |-> {}]
        /\ objects = {}
        /\ attached = {}
        /\ visits = [key \in Keys |-> 0]
        /\ history = [id \in Ids |-> {}]

Activate(key) ==
  /\ visits[key] < MaxVisits
  /\ ~\E id \in active : id[1] = key
  /\ active' = active \cup {<<key, visits[key] + 1>>}
  /\ visits' = [visits EXCEPT ![key] = @ + 1]
  /\ UNCHANGED <<receipt, objects, attached, history>>

RecordActivationActor(id, actor) ==
  /\ id \in active
  /\ receipt[id] = {}
  /\ actor \in {id} \X {1, 2}
  /\ receipt' = [receipt EXCEPT ![id] = {actor}]
  /\ history' = [history EXCEPT ![id] = @ \cup {actor}]
  /\ objects' = objects \cup {actor}
  /\ UNCHANGED <<active, attached, visits>>

Reattach(id) ==
  /\ id \in active
  /\ receipt[id] # {}
  /\ attached' = attached \cup {id}
  /\ UNCHANGED <<active, receipt, objects, visits, history>>

Restart == /\ attached' = {}
           /\ UNCHANGED <<active, receipt, objects, visits, history>>

Complete(id) ==
  /\ id \in attached
  /\ active' = active \ {id}
  /\ attached' = attached \ {id}
  /\ UNCHANGED <<receipt, objects, visits, history>>

Next == \/ \E key \in Keys : Activate(key)
        \/ \E id \in Ids : (\E actor \in {id} \X {1, 2} : RecordActivationActor(id, actor)) \/ Reattach(id) \/ Complete(id)
        \/ Restart
Spec == Init /\ [][Next]_vars
OneActivationPerKey == \A key \in Keys : Cardinality({id \in active : id[1] = key}) <= 1
OneActorPerActivation == \A id \in Ids : Cardinality(history[id]) <= 1
NoOrphanActor == objects = UNION {receipt[id] : id \in Ids}
AttachedActorExists == attached \subseteq active /\ (\A id \in attached : receipt[id] # {} /\ receipt[id] \subseteq objects)
=============================================================================
