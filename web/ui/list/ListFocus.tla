---- MODULE ListFocus ----
EXTENDS Naturals
\* List's DOM focus fallback, below FileList's selection model. A folder can
\* reopen empty with persisted focus on a preceding row. Starting its inline
\* editor mounts/focuses an input; index reconciliation then clears that stale
\* focus, and List's passive effect returns keyboard focus to its container.
\* UnixFS's blank-name onBlur cancels the editor if the fallback takes focus.
\* ListRow already preserves editable focus; List must use the same rule.
\* Other callers (ordinary lists and Git lists) use the same fallback; the
\* no-editor branch models their keyboard target when a focused row disappears.
\* Rows and document focus are bounded to the one row relevant to this race.
CONSTANT PreserveEditor
VARIABLES phase, editor, active
vars == <<phase, editor, active>>

Init ==
  /\ phase = "empty-stale-index"
  /\ editor = FALSE
  /\ active = "list"

\* The person starts a folder; its ref focuses the mounted input.
StartEditor ==
  /\ phase = "empty-stale-index"
  /\ phase' = "reconcile"
  /\ editor' = TRUE
  /\ active' = "editor"

\* A normal list can lose its focused row with no inline editor.
RemoveRow ==
  /\ phase = "empty-stale-index"
  /\ phase' = "reconcile"
  /\ editor' = FALSE
  /\ active' = "body"

\* translateIndicesToNewOrder clears the index of a row no longer present.
Reconcile ==
  /\ phase = "reconcile"
  /\ phase' = "fallback"
  /\ UNCHANGED <<editor, active>>

\* List's effect runs after the input's focus ref and the index update.
Fallback ==
  /\ phase = "fallback"
  /\ phase' = "done"
  /\ active' = IF PreserveEditor /\ active = "editor" THEN active ELSE "list"
  /\ UNCHANGED editor

Done == phase = "done" /\ UNCHANGED vars
Next == StartEditor \/ RemoveRow \/ Reconcile \/ Fallback \/ Done

EditorKeepsFocus == editor => active = "editor"
KeyboardTargetRestored == phase = "done" /\ ~editor => active = "list"
====
