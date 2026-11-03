----------------------------- MODULE Tickets -----------------------------
\* ServeTicket creates a bearer ticket for the signed admitted peer.
\* ServeSocket removes it before accepting a socket, including failed accepts.
\* Two tokens and peers bound the model; cryptographic verification and the
\* transport RPC exchange are checked by TestSignaling.
EXTENDS Naturals, FiniteSets
VARIABLES state, owner, connected, redeemed
Tokens == {1, 2}
Peers == {1, 2}
vars == <<state, owner, connected, redeemed>>
Init == /\ state = [t \in Tokens |-> "absent"]
        /\ owner = [t \in Tokens |-> 0] /\ connected = {}
        /\ redeemed = [t \in Tokens |-> 0]
Issue(t, p) == /\ state[t] = "absent" /\ p \in Peers
               /\ state' = [state EXCEPT ![t] = "issued"]
               /\ owner' = [owner EXCEPT ![t] = p]
               /\ UNCHANGED <<connected, redeemed>>
Expire(t) == /\ state[t] = "issued"
             /\ state' = [state EXCEPT ![t] = "expired"]
             /\ UNCHANGED <<owner, connected, redeemed>>
Redeem(t) == /\ state[t] = "issued"
             /\ state' = [state EXCEPT ![t] = "spent"]
             /\ redeemed' = [redeemed EXCEPT ![t] = @ + 1]
             /\ connected' = connected \cup {t}
             /\ UNCHANGED owner
RejectHandshake(t) == /\ state[t] = "issued"
                      /\ state' = [state EXCEPT ![t] = "spent"]
                      /\ UNCHANGED <<owner, connected, redeemed>>
Close(t) == /\ t \in connected /\ connected' = connected \ {t}
            /\ UNCHANGED <<state, owner, redeemed>>
Next == (\E t \in Tokens : (\E p \in Peers : Issue(t,p)) \/ Expire(t)
         \/ Redeem(t) \/ RejectHandshake(t) \/ Close(t)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
SingleUse == \A t \in Tokens : redeemed[t] <= 1
BoundIdentity == \A t \in connected : owner[t] \in Peers /\ state[t] = "spent"
=============================================================================
