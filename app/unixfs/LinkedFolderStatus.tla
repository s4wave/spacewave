---- MODULE LinkedFolderStatus ----
\* What the Drive view and the sync status button draw for a linked folder,
\* over LinkedFolderJourney.
\*
\* The linker is Alice on the linking Device: its Drive view shows the
\* node's indexing progress and a rejected root with Reset, and its sync
\* status button shows the cloud upload queue's hold, which only the session
\* holding that queue knows. The owner is Alice on her laptop: her row shows
\* where the file can be read from, and the laptop's "keep a full copy"
\* switch, which the account stores per Device, turns its account copy on;
\* KeepFullCopy specifies the switch's own write. The member is Bob: his row
\* shows whether the file can be read and, when it cannot, that its holders
\* are offline. Nobody without a session sees a Space, so no such person is
\* modeled.
EXTENDS LinkedFolderJourney

\* Alice on her laptop holds the switch even when her row is not modeled.
People == {"linker", "owner"} \cup Readers

Buttons == {"open", "keepOn", "keepOff", "resetRoot", "manageBilling"}

Shown(p) ==
  IF p = "linker"
    THEN (IF online /\ linked = "rejected" THEN {"resetRoot"} ELSE {})
      \cup (IF online /\ hold # "none" THEN {"manageBilling"} ELSE {})
    ELSE IF ~Active(p) THEN {}
    ELSE (IF p \in Readers /\ file[p] \in {"placeholder", "present", "unavailable"}
            THEN {"open"} ELSE {})
      \cup (IF p = "owner" THEN {IF keep THEN "keepOff" ELSE "keepOn"} ELSE {})

\* The provider of each Device enforces its own rules: the node's Device
\* resets its root, the queue's session reaches billing, and the laptop's
\* provider stores the laptop's setting.
Allowed(p, b) ==
  \/ b = "open" /\ p \in Readers /\ Active(p)
     /\ file[p] \in {"placeholder", "present", "unavailable"}
  \/ b = "keepOn" /\ p = "owner" /\ lapOnline /\ ~keep
  \/ b = "keepOff" /\ p = "owner" /\ lapOnline /\ keep
  \/ b = "resetRoot" /\ p = "linker" /\ online /\ linked = "rejected"
  \/ b = "manageBilling" /\ p = "linker" /\ hold # "none"

\* A row's words: where the file is, or why it cannot be read now, from what
\* the reader observes. A reachable linking Device whose file moved on
\* answers that its version is superseded; an edit not yet published is not
\* visible to anyone else. The row names the Devices that hold an unavailable
\* file.
RowHint(r) ==
  CASE file[r] = "none" ->
         IF linked = "indexing" THEN "The owner's Device is indexing the folder"
         ELSE "Nothing published yet"
    [] file[r] = "filling" -> "Opening"
    [] file[r] = "present" /\ have[r] < pub -> "On this device; newer version on its way"
    [] file[r] = "present" -> "On this device"
    [] Serves(r, pub) -> "Available from its holders"
    [] online /\ linked # "rejected" -> "A newer version is on its way"
    [] OTHER -> "Unavailable: no holder can serve it now"

\* The sync status button's words for a stopped cloud upload queue. The
\* browser's own quota keeps its label, which names this browser instead.
HoldLabel == [storage |-> "Spacewave Cloud storage full",
              spending |-> "Spacewave Cloud spending limit reached"]
BrowserQuotaLabel == "Storage full"

Hint(p) ==
  IF p = "linker"
    THEN CASE ~online -> "Indexing resumes when this Device wakes"
           [] linked = "rejected" -> "The folder is on another disk now"
           [] hold # "none" -> HoldLabel[hold]
           [] linked = "indexing" -> "Indexing the folder"
           [] OTHER -> "Linked"
    ELSE IF ~Active(p) THEN "Copies resume when this Device wakes"
    ELSE IF p \notin Readers THEN "Keeps a full copy of this Space"
    ELSE RowHint(p)

PressOpen(p) == p \in Readers /\ "open" \in Shown(p)
  /\ (Open(p) \/ (file[p] = "present" /\ UNCHANGED jvars))
PressReset(p) == "resetRoot" \in Shown(p) /\ Reset
PressKeep(p) == Shown(p) \cap {"keepOn", "keepOff"} # {} /\ SetKeep
\* Billing is another screen; Released is its outcome.
PressBilling(p) == "manageBilling" \in Shown(p) /\ UNCHANGED jvars

StatusNext ==
  \/ Edit \/ Index \/ Indexed \/ Reject \/ Sleep \/ Wake
  \/ Upload \/ Held \/ Released \/ LaptopSleep \/ LaptopWake \/ Backfill \/ Evict
  \/ \E r \in Readers :
       Appear(r) \/ Filled(r) \/ Retarget(r) \/ NoHolder(r) \/ Update(r)
  \/ \E p \in People :
       PressOpen(p) \/ PressReset(p) \/ PressKeep(p) \/ PressBilling(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Only the Device that links the folder resets its root, and only the session
\* holding the upload queue reaches billing from its hold.
LinkerOnly == \A p \in People \ {"linker"} :
  Shown(p) \cap {"resetRoot", "manageBilling"} = {}
\* A file is offered only once published.
OpenOnlyPublished == \A p \in People : "open" \in Shown(p) => pub > 0
\* A stopped upload queue is named on the linking Device, unless a rejected
\* root needs Reset first, and never as the browser's quota.
HoldNamed ==
  /\ online /\ hold # "none" /\ linked # "rejected" =>
       Hint("linker") = HoldLabel[hold]
  /\ \A h \in DOMAIN HoldLabel : HoldLabel[h] # BrowserQuotaLabel
\* A row says the file is unavailable only when no holder serves it.
HintTruthful == \A r \in Readers :
  RowHint(r) = "Unavailable: no holder can serve it now" => ~Serves(r, pub)
====
