---- MODULE ResourceHooks ----
\* What a resource hook reports for one child of one parent resource, as
\* useResource and useStreamingResource hold it in resourceTransitionState.
\*
\* A hook reports one of three states. Loading may show the value it had.
\* Ready shows the value of the current deps, or no value when the source
\* settled empty. Failed shows no value: only the last value of the current
\* deps, which may be stale, with Retry. A failed parent fails the child,
\* which keeps its value as last and runs nothing until the parent is ready.
\* A failure that kept a last stays failed over it through a parent reload,
\* the parent's recovery and the run that follows, until the run yields, so
\* recovery never blanks kept content. A parent reload keeps the value. A deps change starts loading with no
\* value and no last, unless the hook keeps the value while loading, as a
\* tree expanding a level does: then the previous deps' value shows only
\* while loading, and never becomes last.
\*
\* A reconnecting stream that fails after a value reopens after a backoff,
\* loading over its value. Once the backoff reaches its cap it reports failed
\* over the value as last, with Retry, and keeps reopening quietly; a value
\* returns it to ready. Retry in either hook reads again at once, staying
\* failed over a kept last. A value that holds a handle the hook released
\* never becomes last.

EXTENDS Naturals

\* MaxKey bounds the deps changes; Cap is the failure count at which the
\* backoff reaches its maximum wait.
MaxKey == 2
Cap == 2

VARIABLES
  key,          \* the current deps; a value is tagged with the deps it read
  carry,        \* the hook keeps the value while new deps load
  handles,      \* values hold handles the hook releases
  reconnect,    \* the stream reopens after failing past a value
  parent,       \* the parent: loading, ready or failed
  status,       \* the hook's own state: loading, ready or failed
  val,          \* the deps the held value read, or 0 for no value
  valRel,       \* the held value's handle was released
  keepable,     \* the held value may become last
  last,         \* the deps the last value read, or 0 for none
  lastRel,      \* the last value's handle was released
  stream,       \* the run: held until the hook's effect starts it, open,
                \* backoff waiting to reopen, or done
  emitted,      \* the open run yielded a value
  fails         \* consecutive failures since the last value, up to Cap;
                \* above 0 the stream failed past a value and reopens
vars == <<key, carry, handles, reconnect, parent, status, val, valRel,
          keepable, last, lastRel, stream, emitted, fails>>

\* The stream failed past a value and reopens.
Reconnecting == fails > 0

\* The source settled with no value.
Empty == stream = "done" /\ status = "ready"

\* RStatus is what the hook reports: an error wins over loading.
RStatus ==
  IF parent = "failed" \/ status = "failed" THEN "failed"
  ELSE IF parent = "loading" \/ status = "loading" THEN "loading"
  ELSE "ready"

\* LastOf is the value a failure keeps as last: the held value if it may be
\* kept and holds no released handle, else the last it had.
LastOf ==
  IF val = 0 THEN last
  ELSE IF keepable /\ ~valRel THEN val ELSE 0
LastOfRel == IF val = 0 THEN lastRel ELSE LastOf # 0 /\ valRel

\* The reported value and last, as resourceOf builds them.
RValue == IF RStatus = "failed" THEN 0 ELSE val
RLast == IF RStatus = "failed" THEN LastOf ELSE 0

\* Content is what a viewer draws: the value, or a failure's last.
Content == IF RStatus = "failed" THEN RLast ELSE RValue
ContentRel == IF RStatus = "failed" THEN LastOfRel ELSE valRel

TypeOK ==
  /\ key \in 1..MaxKey
  /\ carry \in BOOLEAN /\ handles \in BOOLEAN /\ reconnect \in BOOLEAN
  /\ parent \in {"loading", "ready", "failed"}
  /\ status \in {"loading", "ready", "failed"}
  /\ val \in 0..MaxKey /\ last \in 0..MaxKey
  /\ valRel \in BOOLEAN /\ lastRel \in BOOLEAN /\ keepable \in BOOLEAN
  /\ stream \in {"held", "open", "backoff", "done"}
  /\ emitted \in BOOLEAN /\ fails \in 0..Cap

Init ==
  /\ key = 1
  /\ carry \in BOOLEAN /\ handles \in BOOLEAN /\ reconnect \in BOOLEAN
  \* Only a reconnecting query of plain data keeps its value across deps,
  \* as the World queries that use the option are.
  /\ carry => reconnect /\ ~handles
  /\ parent = "loading" /\ status = "loading"
  /\ val = 0 /\ valRel = FALSE /\ keepable = TRUE
  /\ last = 0 /\ lastRel = FALSE
  /\ stream = "held" /\ emitted = FALSE /\ fails = 0

Fixed == UNCHANGED <<key, carry, handles, reconnect>>

\* ToLast moves the value to last and sets the hook's state to st.
ToLast(st) ==
  /\ val' = 0 /\ valRel' = FALSE /\ keepable' = TRUE
  /\ last' = LastOf /\ lastRel' = LastOfRel
  /\ status' = st

\* Wait sets the state a run or a wait starts from: a failure that kept a
\* last, or a quiet one, stays failed; anything else loads.
Wait(q) ==
  status' = IF status = "failed" /\ (q \/ LastOf # 0) THEN "failed"
            ELSE "loading"

\* Begin opens a run from Wait(q).
Begin(q) ==
  /\ stream' = "open" /\ emitted' = FALSE
  /\ Wait(q)

\* The parent turns ready; the child renders once before its effect runs.
ParentReady ==
  /\ parent \in {"loading", "failed"}
  /\ parent' = "ready"
  /\ UNCHANGED <<status, val, valRel, keepable, last, lastRel, stream,
                 emitted, fails>>
  /\ Fixed

\* The hook's effect starts the held run on a ready parent. After a parent
\* failure the child starts failed over its kept last.
Start ==
  /\ parent = "ready" /\ stream = "held"
  /\ Begin(FALSE)
  /\ UNCHANGED <<parent, val, valRel, keepable, last, lastRel, fails>>
  /\ Fixed

\* A parent reload keeps the value while the child waits for it.
ParentLoad ==
  /\ parent = "ready"
  /\ parent' = "loading"
  /\ stream' = "held" /\ Wait(FALSE)
  /\ UNCHANGED <<val, valRel, keepable, last, lastRel, emitted, fails>>
  /\ Fixed

\* A failed parent fails the child over its value as last; the child waits
\* for the parent rather than settling with no value.
ParentFail ==
  /\ parent \in {"loading", "ready"}
  /\ parent' = "failed"
  /\ stream' = "held"
  /\ ToLast("failed")
  /\ UNCHANGED <<emitted, fails>>
  /\ Fixed

ParentRetry ==
  /\ parent = "failed"
  /\ parent' = "loading"
  /\ Wait(FALSE)
  /\ UNCHANGED <<val, valRel, keepable, last, lastRel, stream, emitted,
                 fails>>
  /\ Fixed

\* The run yields a value of the current deps.
Emit ==
  /\ parent = "ready" /\ stream = "open"
  /\ val' = key /\ valRel' = FALSE /\ keepable' = TRUE
  /\ last' = 0 /\ lastRel' = FALSE
  /\ status' = "ready" /\ emitted' = TRUE /\ fails' = 0
  /\ UNCHANGED <<parent, stream>>
  /\ Fixed

\* The run ends before its first value: the source holds nothing.
EndEmpty ==
  /\ parent = "ready" /\ stream = "open" /\ ~emitted
  /\ stream' = "done"
  /\ val' = 0 /\ valRel' = FALSE /\ keepable' = TRUE
  /\ last' = 0 /\ lastRel' = FALSE
  /\ status' = "ready"
  /\ UNCHANGED <<parent, emitted, fails>>
  /\ Fixed

\* The run fails. A reconnecting stream past a value backs off, loading over
\* the value until the backoff reaches its cap, then failed over it as last.
StreamFail ==
  /\ parent = "ready" /\ stream = "open"
  /\ IF reconnect /\ (emitted \/ Reconnecting)
       THEN /\ stream' = "backoff"
            /\ fails' = IF fails < Cap THEN fails + 1 ELSE Cap
            /\ IF fails + 1 >= Cap
                 THEN ToLast("failed")
                 ELSE /\ status' = "loading"
                      /\ UNCHANGED <<val, valRel, keepable, last, lastRel>>
       ELSE /\ stream' = "done" /\ ToLast("failed")
            /\ UNCHANGED fails
  /\ UNCHANGED <<parent, emitted>>
  /\ Fixed

\* The backoff ends and the run reopens, quietly once capped.
Reopen ==
  /\ parent = "ready" /\ stream = "backoff"
  /\ Begin(fails >= Cap)
  /\ UNCHANGED <<parent, val, valRel, keepable, last, lastRel, fails>>
  /\ Fixed

\* Retry in either hook reads again at once, staying failed over a kept last.
Retry ==
  /\ parent = "ready" /\ status = "failed"
  /\ Begin(FALSE)
  /\ UNCHANGED <<parent, val, valRel, keepable, last, lastRel, fails>>
  /\ Fixed

\* The hook releases the handle a value holds: a held value may stay while
\* loading, but never becomes last; a last value goes.
Release ==
  /\ handles
  /\ \/ /\ val # 0 /\ ~valRel
        /\ valRel' = TRUE /\ keepable' = FALSE
        /\ UNCHANGED <<last, lastRel>>
     \/ /\ val = 0 /\ last # 0
        /\ last' = 0 /\ lastRel' = FALSE
        /\ UNCHANGED <<valRel, keepable>>
  /\ UNCHANGED <<parent, status, val, stream, emitted, fails>>
  /\ Fixed

\* The deps change: the hook starts loading the new deps with no last, and
\* no value unless it keeps the previous one while loading.
DepsChange ==
  /\ key < MaxKey
  /\ key' = key + 1
  /\ IF carry /\ val # 0
       THEN /\ keepable' = FALSE /\ UNCHANGED <<val, valRel>>
       ELSE /\ val' = 0 /\ valRel' = FALSE /\ keepable' = TRUE
  /\ last' = 0 /\ lastRel' = FALSE
  /\ status' = "loading"
  /\ stream' = "held"
  /\ emitted' = FALSE /\ fails' = 0
  /\ UNCHANGED <<carry, handles, reconnect, parent>>

Next ==
  \/ ParentReady \/ Start \/ ParentLoad \/ ParentFail \/ ParentRetry
  \/ Emit \/ EndEmpty \/ StreamFail \/ Reopen \/ Retry
  \/ Release \/ DepsChange

\* A failed resource reports no value: a kept value is readable only as last.
NoValueBesideError == RStatus = "failed" => RValue = 0

\* A released handle is never a resource's last value.
NoReleasedHandleKept == RLast # 0 => ~LastOfRel

\* The reported value read the current deps, except a value kept while new
\* deps load, which shows only while loading.
KeptValueIsThisKey ==
  RValue # 0 => (val = key \/ (carry /\ RStatus = "loading"))

\* last always read the current deps: it never crosses a deps change.
LastIsThisKey == RLast # 0 => RLast = key

\* A child runs only on a ready parent.
NoChildOnFailedParent == parent # "ready" => stream # "open"

\* Every failed state offers Retry.
FailedHasRetry ==
  RStatus = "failed" =>
    \/ ENABLED Retry \/ ENABLED ParentRetry
    \* A reloading parent is the Retry under way; the child follows it.
    \/ parent = "loading"

\* A reconnecting stream past its cap reports failed.
CappedReportsFailed == stream = "backoff" /\ fails >= Cap => RStatus = "failed"

\* Ready with no value only when the source settled empty: never a failure
\* or a wait read as empty.
NoFalseEmpty == RStatus = "ready" /\ RValue = 0 => Empty

\* Content of the current deps stays through the stream's own failures, the
\* cap, Retry, and a parent's failure, reload and recovery, unless its
\* handle was released.
OwnFailureKeepsContent ==
  [][(Content = key /\ ~ContentRel /\
      (StreamFail \/ Reopen \/ Retry \/ ParentFail \/ ParentLoad \/
       ParentRetry \/ ParentReady \/ Start))
       => Content' # 0]_vars

\* A run with nothing kept shows its wait: a run opens failed only over a
\* kept last, or as the capped stream's quiet reopen.
NothingKeptLoads ==
  [][(stream # "open" /\ stream' = "open" /\ status' = "failed")
       => (LastOf' # 0 \/ fails >= Cap)]_vars

\* A value returns a failed hook to ready.
ValueRecovers == [][RStatus = "failed" /\ Emit => RStatus' = "ready"]_vars

\* A deps change drops value and last in the same step, kept value aside.
DepsChangeDropsLast ==
  [][DepsChange => (RLast' = 0 /\ (~carry => RValue' = 0))]_vars

\* Witnesses of the traces the model must reach; each mutant negates one.
FailThenRecover == RStatus = "failed" /\ ENABLED Emit
FailKeptThenDeps == RStatus = "failed" /\ RLast # 0 /\ ENABLED DepsChange
CappedWithRetry ==
  stream = "backoff" /\ fails >= Cap /\ RLast # 0 /\ ENABLED Retry
ReleaseUnderKept ==
  RStatus = "failed" /\ RLast # 0 /\ ENABLED Release
ParentFailsOverValue == parent = "failed" /\ RLast # 0
RecoversOverKept ==
  parent = "ready" /\ stream = "open" /\ RStatus = "failed" /\ RLast # 0
CarriedWhileLoading == RStatus = "loading" /\ val # 0 /\ val # key
Unreached == TRUE

====
