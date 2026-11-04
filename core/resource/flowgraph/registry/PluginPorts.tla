--------------------------- MODULE PluginPorts ---------------------------
EXTENDS Naturals, FiniteSets

\* The Device compiler binds lookup configs to one registration; the bridge
\* never selects an installation by service ID alone. Session ServeAuthorized
\* precedes every invocation, and registration cancellation ends active calls.
\* Models the Device fixture and node controllers using the retained capability.
\* SRPC framing and link authentication belong to their existing libraries.
CONSTANTS Registrations, Peers, Allowed
VARIABLES registered, compiled, phase, route, granted
vars == <<registered, compiled, phase, route, granted>>

Init == /\ registered = {}
        /\ compiled = {}
        /\ phase = [p \in Peers |-> "idle"]
        /\ route = [p \in Peers |-> CHOOSE r \in Registrations : TRUE]
        /\ granted = {}

Register(r) == /\ r \notin registered
               /\ registered' = registered \cup {r}
               /\ UNCHANGED <<compiled, phase, route, granted>>

Compile(r) == /\ r \in registered
              /\ compiled' = compiled \cup {r}
              /\ UNCHANGED <<registered, phase, route, granted>>

Open(p, r) == /\ phase[p] = "idle"
              /\ r \in compiled
              /\ phase' = [phase EXCEPT ![p] = "waiting"]
              /\ route' = [route EXCEPT ![p] = r]
              /\ UNCHANGED <<registered, compiled, granted>>

Admit(p) == /\ phase[p] = "waiting"
            /\ p \in Allowed
            /\ phase' = [phase EXCEPT ![p] = "admitted"]
            /\ granted' = granted \cup {p}
            /\ UNCHANGED <<registered, compiled, route>>

Refuse(p) == /\ phase[p] = "waiting"
             /\ p \notin Allowed
             /\ phase' = [phase EXCEPT ![p] = "closed"]
             /\ UNCHANGED <<registered, compiled, route, granted>>

Invoke(p) == /\ phase[p] = "admitted"
             /\ route[p] \in registered
             /\ phase' = [phase EXCEPT ![p] = "serving"]
             /\ UNCHANGED <<registered, compiled, route, granted>>

Withdraw(r) == /\ r \in registered
               /\ registered' = registered \ {r}
               /\ UNCHANGED <<compiled, phase, route, granted>>

Close(p) == /\ phase[p] \in {"waiting", "admitted", "serving"}
            /\ phase' = [phase EXCEPT ![p] = "closed"]
            /\ UNCHANGED <<registered, compiled, route, granted>>

Next == (\E r \in Registrations : Register(r) \/ Compile(r) \/ Withdraw(r))
        \/ (\E p \in Peers, r \in Registrations : Open(p, r))
        \/ (\E p \in Peers : Admit(p) \/ Refuse(p) \/ Invoke(p) \/ Close(p))

TypeOK == /\ registered \subseteq Registrations
          /\ compiled \subseteq Registrations
          /\ phase \in [Peers -> {"idle", "waiting", "admitted", "serving", "closed"}]
          /\ route \in [Peers -> Registrations]
          /\ granted \subseteq Peers
Admission == \A p \in Peers : phase[p] = "serving" => p \in Allowed /\ p \in granted
Routing == \A p \in Peers : phase[p] = "serving" => route[p] \in compiled
Spec == Init /\ [][Next]_vars
=============================================================================
