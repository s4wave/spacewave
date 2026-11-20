---- MODULE KeepFullCopy ----
\* The "keep a full copy" switch for one Device of Alice's account. The
\* account stores the setting per Device; the switch shows the stored value
\* and writes through the laptop's provider. Pressed, it shows the requested
\* value as saving until the write returns, then the stored value; a failed
\* write keeps the stored value and offers Retry. Another session of the
\* account may change the setting at any time, and the switch follows the
\* watched value. The switch is drawn on the Device's own screens, so a
\* Device that sleeps takes the whole screen with it and is not modeled.
CONSTANTS Faults \* a subset of {"saveFails", "otherWriter"}

People == {"owner"}
Buttons == {"keepOn", "keepOff", "retry"}

VARIABLES
  keep,   \* the stored setting
  sw,     \* the switch's write: idle, saving or failed
  target  \* the value the last press asked for
vars == <<keep, sw, target>>

Init == keep = FALSE /\ sw = "idle" /\ target = FALSE

Shown(p) ==
  IF sw = "saving" THEN {}
  ELSE {IF keep THEN "keepOff" ELSE "keepOn"}
    \cup (IF sw = "failed" THEN {"retry"} ELSE {})

Allowed(p, b) ==
  /\ sw # "saving"
  /\ \/ b = "keepOn" /\ ~keep
     \/ b = "keepOff" /\ keep
     \/ b = "retry" /\ sw = "failed"

Hint(p) ==
  CASE sw = "saving" -> IF target THEN "Turning on" ELSE "Turning off"
    [] sw = "failed" -> "Could not save the setting"
    [] keep -> "This Device keeps a full copy"
    [] OTHER -> "This Device fetches files when opened"

Press(p) == \E b \in Shown(p) \ {"retry"} :
  sw' = "saving" /\ target' = (b = "keepOn") /\ UNCHANGED keep
Retry(p) == "retry" \in Shown(p) /\ sw' = "saving" /\ UNCHANGED <<keep, target>>
Saved == sw = "saving" /\ sw' = "idle" /\ keep' = target /\ UNCHANGED target
SaveFails == "saveFails" \in Faults /\ sw = "saving" /\ sw' = "failed"
  /\ UNCHANGED <<keep, target>>
\* Another session writes; a pending write lands after it, last writer wins.
OtherWriter == "otherWriter" \in Faults /\ keep' = ~keep /\ UNCHANGED <<sw, target>>

Next ==
  \/ Saved \/ SaveFails \/ OtherWriter
  \/ \E p \in People : Press(p) \/ Retry(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

TypeOK ==
  /\ Faults \subseteq {"saveFails", "otherWriter"}
  /\ keep \in BOOLEAN /\ target \in BOOLEAN
  /\ sw \in {"idle", "saving", "failed"}
\* The switch shows one value at a time.
OneValue == \A p \in People : ~({"keepOn", "keepOff"} \subseteq Shown(p))
\* Outside a write, the switch shows the stored value, also after a failure.
ShowsStored == \A p \in People :
  sw # "saving" => Shown(p) \ {"retry"} = {IF keep THEN "keepOff" ELSE "keepOn"}
====
