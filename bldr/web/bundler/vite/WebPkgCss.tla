---------------------------- MODULE WebPkgCss ----------------------------
EXTENDS Integers, FiniteSets

\* Browser ESM consumers receive a module only after Vite's CSS preload.
\* Server consumers need the same exports without starting a DOM operation.
CONSTANTS Browser, AwaitCss, AwaitGraph
VARIABLES phase, css, compiled, sheet
vars == <<phase, css, compiled, sheet>>
Modules == {"entry", "lazy"}
Init == /\ phase = "building" /\ css = "absent"
        /\ compiled = {} /\ sheet = {}
Compile(m) == /\ phase = "building" /\ m \in Modules \ compiled
              /\ compiled' = compiled \cup {m}
              /\ UNCHANGED <<phase, css, sheet>>
Extract == /\ phase = "building" /\ (~AwaitGraph \/ compiled = Modules)
           /\ sheet' = compiled /\ phase' = "idle"
           /\ UNCHANGED <<css, compiled>>
Import == /\ phase = "idle" /\ phase' = "loading"
          /\ css' = IF Browser THEN "pending" ELSE "unneeded"
          /\ UNCHANGED <<compiled, sheet>>
CssLoaded == /\ phase = "loading" /\ css = "pending"
             /\ css' = "loaded" /\ UNCHANGED <<phase, compiled, sheet>>
CssFailed == /\ phase = "loading" /\ css = "pending"
             /\ phase' = "failed" /\ css' = "failed"
             /\ UNCHANGED <<compiled, sheet>>
Publish == /\ phase = "loading"
           /\ (~AwaitCss \/ css \in {"loaded", "unneeded"})
           /\ phase' = "ready" /\ UNCHANGED <<css, compiled, sheet>>
Reload == /\ phase = "failed" /\ phase' = "idle" /\ css' = "absent"
          /\ UNCHANGED <<compiled, sheet>>
Next == (\E m \in Modules : Compile(m)) \/ Extract \/ Import \/ CssLoaded \/ CssFailed \/ Publish \/ Reload \/ UNCHANGED vars
TypeOK == /\ phase \in {"building", "idle", "loading", "ready", "failed"}
          /\ css \in {"absent", "pending", "loaded", "unneeded", "failed"}
          /\ compiled \subseteq Modules /\ sheet \subseteq Modules
CompleteSheet == phase # "building" => sheet = Modules
ReadyStyled == phase = "ready" => css \in {"loaded", "unneeded"}
ServerHasNoCss == ~Browser => css \in {"absent", "unneeded"}
Spec == Init /\ [][Next]_vars
=============================================================================
