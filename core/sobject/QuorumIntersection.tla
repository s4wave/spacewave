---- MODULE QuorumIntersection ----
EXTENDS Naturals, FiniteSets
(* rounds.go newRoundTally/addVoter/quorum/commit and config HasQuorum.
   ReceivePrecommit models a validated message arriving, including retransmits:
   addVoter's peer-keyed map deduplicates before weightOf sums VotingWeight.
   Four voters have weights 2,1,1,2, total six; voter four may equivocate and
   holds exactly one third. Honest voters sign one value per height/round;
   repeated arrivals of that value are permitted. Six arrivals and two values
   bound the exploration. Commit follows HasQuorum's strict 3*w > 2*total.
   ControlDriver/stepRound consumes these tallies. Signatures and participant
   validation precede receipt and are assumptions; arbitrary weights, cross-
   round locks, persistence and recovery remain outside this model. Lean
   Rounds proves unbounded weighted agreement; this model exercises finite
   duplicate arrivals and the Go tally's weightOf calculation. weight is a
   ghost observation of weightOf after each insertion, not a cache in Go. *)
CONSTANTS StrictQuorum, Deduplicate
VARIABLES signed, voters, weight, received, committed
vars == <<signed, voters, weight, received, committed>>
Values == {1, 2}
Voters == 1..4
VotingWeight(a) == IF a \in {1, 4} THEN 2 ELSE 1
Init == /\ signed = [a \in Voters |-> {}]
        /\ voters = [v \in Values |-> {}]
        /\ weight = [v \in Values |-> 0]
        /\ received = 0 /\ committed = {}
ReceivePrecommit(a, value) ==
  /\ received < 6
  /\ (a = 4 \/ signed[a] = {} \/ value \in signed[a])
  /\ signed' = [signed EXCEPT ![a] = @ \cup {value}]
  /\ voters' = [voters EXCEPT ![value] = @ \cup {a}]
  /\ weight' = [weight EXCEPT ![value] = @ +
       (IF Deduplicate /\ a \in voters[value] THEN 0 ELSE VotingWeight(a))]
  /\ received' = received + 1 /\ UNCHANGED committed
HasQuorum(value) == IF StrictQuorum THEN 3 * weight[value] > 2 * 6
                   ELSE 3 * weight[value] >= 2 * 6
Commit(value) == /\ HasQuorum(value)
                 /\ committed' = committed \cup {value}
                 /\ UNCHANGED <<signed, voters, weight, received>>
Next == (\E a \in Voters, v \in Values: ReceivePrecommit(a, v))
        \/ (\E v \in Values: Commit(v)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
Agreement == Cardinality(committed) <= 1
WeightMatchesSigners == \A v \in Values:
  weight[v] = Cardinality(voters[v]) + Cardinality(voters[v] \cap {1, 4})
====
