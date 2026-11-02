---- MODULE ManifestPayload ----
(* CommitManifest builds a bundle outside the World tree, then StoreManifestOp
   references its root. World publication stores declared payloads and keeps
   derived World pages in the mounted engine. Replay can read those pages
   while mounted, but an unmount loses them; its saved operation must retain
   the bundle as a payload. One operation and one bundle represent the same
   root classification in direct commit and replay. External bundles already
   durable are included as an initial case. DeclaresPayload=FALSE models the
   omitted StoreManifestOp.GetPayloadRefs method. *)
CONSTANT DeclaresPayload
VARIABLES phase, mounted, volatileBundle, durableBundle, missing
vars == <<phase, mounted, volatileBundle, durableBundle, missing>>
Init == /\ phase = "built" /\ mounted = TRUE /\ volatileBundle = TRUE
        /\ durableBundle \in BOOLEAN /\ missing = FALSE
Commit == /\ phase = "built" /\ phase' = "committed"
          /\ durableBundle' = (durableBundle \/ DeclaresPayload)
          /\ UNCHANGED <<mounted, volatileBundle, missing>>
Unmount == /\ phase = "committed" /\ mounted
           /\ mounted' = FALSE /\ volatileBundle' = FALSE
           /\ UNCHANGED <<phase, durableBundle, missing>>
Replay == /\ phase = "committed" /\ ~mounted
          /\ phase' = "replayed" /\ mounted' = TRUE
          /\ missing' = ~durableBundle
          /\ UNCHANGED <<volatileBundle, durableBundle>>
Next == Commit \/ Unmount \/ Replay \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
ReplayRetainsBundle == ~missing
====
