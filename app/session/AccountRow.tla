---- MODULE AccountRow ----
\* One account row in the session picker. A person picks the account, or names
\* an unnamed local account in place. The row derives what it shows from the
\* watched session metadata and one edit state, never from separate flags.
\*
\* Kinds: a cloud account shows its username and has no name of its own. A
\* local account opens without a PIN, so the picker mounts it on Save and calls
\* SetDisplayName. A PIN local account cannot be mounted from the picker, so
\* it is named after unlocking, in its own settings.

People == {"person"}
Kinds == {"cloud", "local", "pin"}
Names == {"", "a", "b"}  \* "" is unnamed; a blank save clears the name.
Buttons == {"select", "name", "type", "save", "cancel"}

VARIABLES
  kind,    \* the account's kind, fixed for the row's life
  loaded,  \* whether the metadata watch has answered
  name,    \* the watched display name
  edit,    \* "viewing", "editing", "saving" or "failed"
  draft    \* the text in the name field; carried through saving and failed
vars == <<kind, loaded, name, edit, draft>>

Changed == draft # name
Nameable == kind = "local"

\* Loading: the row shows the session number and can still be picked.
\* Named: the name, with a pencil to rename a local account.
\* Unnamed local: its creation time and account id suffix, with Name.
\* Locked: an unnamed PIN account, with a lock that says it is named after
\* unlocking. Editing, Saving and Save failed show the field in the row.
Shown(p) ==
  CASE ~loaded -> {"select"}
    [] edit = "viewing" -> IF Nameable THEN {"select", "name"} ELSE {"select"}
    [] edit = "editing" -> {"type", "cancel"} \cup (IF Changed THEN {"save"} ELSE {})
    [] edit = "saving" -> {}
    [] edit = "failed" -> {"type", "cancel"} \cup (IF Changed THEN {"save"} ELSE {})

\* SetDisplayName works on a mounted local session; selecting works always.
Allowed(p, b) ==
  \/ b = "select"
  \/ b \in {"name", "type", "cancel"} /\ Nameable
  \/ b = "save" /\ Nameable /\ Changed

\* What the row says with nothing to press.
Hint(p) == IF edit = "saving" THEN "Saving the name" ELSE ""

Init ==
  /\ kind \in Kinds
  /\ loaded = FALSE
  /\ name \in Names
  /\ edit = "viewing"
  /\ draft = ""

Load == ~loaded /\ loaded' = TRUE /\ UNCHANGED <<kind, name, edit, draft>>
Select(p) == "select" \in Shown(p) /\ UNCHANGED vars
Start(p) ==
  /\ "name" \in Shown(p)
  /\ edit' = "editing" /\ draft' = name
  /\ UNCHANGED <<kind, loaded, name>>
Type(p) ==
  /\ "type" \in Shown(p)
  /\ \E d \in Names : d # draft /\ draft' = d
  /\ edit' = "editing"
  /\ UNCHANGED <<kind, loaded, name>>
Cancel(p) ==
  /\ "cancel" \in Shown(p)
  /\ edit' = "viewing" /\ draft' = ""
  /\ UNCHANGED <<kind, loaded, name>>
Save(p) ==
  /\ "save" \in Shown(p)
  /\ edit' = "saving"
  /\ UNCHANGED <<kind, loaded, name, draft>>
\* The write lands and the watch shows it, which ends the save.
Saved ==
  /\ edit = "saving"
  /\ name' = draft /\ edit' = "viewing" /\ draft' = ""
  /\ UNCHANGED <<kind, loaded>>
\* The mount or the write fails; the draft stays for a retry.
SaveFailed ==
  /\ edit = "saving"
  /\ edit' = "failed"
  /\ UNCHANGED <<kind, loaded, name, draft>>
\* Another session of the account, or the CLI, renames it while the row is open.
Renamed ==
  /\ loaded /\ kind # "cloud"
  /\ \E n \in Names : n # name /\ name' = n
  /\ UNCHANGED <<kind, loaded, edit, draft>>

Next ==
  \/ Load \/ Saved \/ SaveFailed \/ Renamed
  \/ \E p \in People : Select(p) \/ Start(p) \/ Type(p) \/ Cancel(p) \/ Save(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Only a local account opening without a PIN offers a name field.
OnlyLocalEdits == kind # "local" => edit = "viewing"
\* A loaded local account always offers its name, named or not.
LocalOffersName ==
  loaded /\ edit = "viewing" /\ kind = "local" => "name" \in Shown("person")
====
