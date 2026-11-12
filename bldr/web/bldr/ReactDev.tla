---- MODULE ReactDev ----
\* Development React: a device setting that boots the app on the development
\* build of React and React DOM instead of the release's production build.
\*
\* Two phases. While the app runs, turning the setting on copies the development
\* files of the release's React version into the service worker's cache, as one
\* generation. At the next boot, an inline script in the shell emits the
\* document's only import map before any module loads: the development map when
\* the setting is on, a service worker controls the page and the cached
\* generation matches the release's React version; the production map otherwise.
\* The service worker answers the development URLs from that generation, or with
\* the production files when the generation is gone, deciding once per document
\* so React and React DOM always come from the same build.

People == {"developer"}
Versions == {"v1", "v2"}
Buttons == {"enable", "disable", "retry", "update", "reload"}

VARIABLES
  flag,       \* the device setting
  cache,      \* "none" or the version of the complete cached generation
  fill,       \* "idle", "fetching" or "failed": the copy into the cache
  release,    \* the release's production React version
  controlled, \* whether a service worker controls this document
  map,        \* "prod" or "dev": the import map the shell emitted at boot
  pick,       \* "", "prod" or "dev": the worker's decision for this document
  react,      \* "", "prod", "dev" or "missing": where react came from
  dom         \* the same for react-dom
vars == <<flag, cache, fill, release, controlled, map, pick, react, dom>>

Loaded == react # "" /\ dom # ""
Running == IF react = "dev" THEN "dev" ELSE "prod"

\* Off: the setting is off; Reload while this document still runs development.
\* Fetching: "Loading development React" while the files copy.
\* Ready: the files match this release; Reload starts development React.
\* On: this document runs development React; the bottom bar shows Dev React.
\* Absent: the copy failed or the cache is gone; production React, with Retry.
\* Mismatch: the cache holds another React version; production, with Update.
State ==
  CASE fill = "fetching" -> "fetching"
    [] ~flag -> "off"
    [] Running = "dev" -> "on"
    [] cache = release -> "ready"
    [] fill = "failed" \/ cache = "none" -> "absent"
    [] OTHER -> "mismatch"

Shown(p) ==
  CASE ~Loaded -> {}
    [] State = "off" -> {"enable"} \cup (IF Running = "dev" THEN {"reload"} ELSE {})
    [] State = "fetching" -> {"disable"}
    [] State = "ready" -> {"reload", "disable"}
    [] State = "on" -> {"disable"}
    [] State = "absent" -> {"retry", "disable"}
    [] State = "mismatch" -> {"update", "disable"}

\* The settings store and the copy enforce these; a reload always works.
Allowed(p, b) ==
  \/ b = "enable" /\ ~flag
  \/ b = "disable" /\ flag
  \/ b \in {"retry", "update"} /\ flag /\ fill # "fetching"
  \/ b = "reload"

Hint(p) ==
  CASE ~Loaded -> "Starting"
    [] State = "fetching" -> "Loading development React"
    [] OTHER -> ""

Init ==
  /\ flag = FALSE /\ cache = "none" /\ fill = "idle"
  /\ release \in Versions /\ controlled \in BOOLEAN
  /\ map = "prod" /\ pick = "" /\ react = "" /\ dom = ""

\* The shell reads the setting, the controller and the cached version
\* synchronously and emits one map. A release update can land before a boot,
\* and a hard reload or a first visit leaves the page without a controller.
Boot ==
  /\ Loaded
  /\ release' \in Versions /\ controlled' \in BOOLEAN
  /\ map' = IF flag /\ controlled' /\ cache = release' THEN "dev" ELSE "prod"
  /\ pick' = "" /\ react' = "" /\ dom' = "" /\ fill' = "idle"
  /\ UNCHANGED <<flag, cache>>

\* One module file loads. A production URL is a static release asset. A
\* development URL reaches the controlling worker, which takes the generation
\* for this document on its first request, or the production files when the
\* generation does not match, and serves every later file from that pick.
Serve(f) ==
  LET from ==
        IF map = "prod" THEN "prod"
        ELSE IF ~controlled THEN "missing"
        ELSE IF pick # "" THEN pick
        ELSE IF cache = release THEN "dev" ELSE "prod"
  IN /\ IF f = "react" THEN react = "" /\ react' = from /\ dom' = dom
                       ELSE dom = "" /\ dom' = from /\ react' = react
     /\ pick' = IF map = "dev" /\ controlled THEN from ELSE pick
     /\ UNCHANGED <<flag, cache, fill, release, controlled, map>>

Enable(p) ==
  /\ "enable" \in Shown(p)
  /\ flag' = TRUE
  /\ fill' = IF cache = release THEN "idle" ELSE "fetching"
  /\ UNCHANGED <<cache, release, controlled, map, pick, react, dom>>
Disable(p) ==
  /\ "disable" \in Shown(p)
  /\ flag' = FALSE /\ fill' = "idle"
  /\ UNCHANGED <<cache, release, controlled, map, pick, react, dom>>
Refetch(p) ==
  /\ \E b \in {"retry", "update"} : b \in Shown(p)
  /\ fill' = "fetching"
  /\ UNCHANGED <<flag, cache, release, controlled, map, pick, react, dom>>
Reload(p) == "reload" \in Shown(p) /\ Boot

\* The copy writes every file of the release's version, then marks the
\* generation complete; a failure leaves the previous generation in place.
Filled ==
  /\ fill = "fetching" /\ fill' = "idle" /\ cache' = release
  /\ UNCHANGED <<flag, release, controlled, map, pick, react, dom>>
FillFailed ==
  /\ fill = "fetching" /\ fill' = "failed"
  /\ UNCHANGED <<flag, cache, release, controlled, map, pick, react, dom>>
\* Clearing site data or a quota eviction drops the cache at any time.
Evict ==
  /\ cache # "none" /\ cache' = "none"
  /\ UNCHANGED <<flag, fill, release, controlled, map, pick, react, dom>>

Next ==
  \/ Boot \/ Filled \/ FillFailed \/ Evict
  \/ \E f \in {"react", "dom"} : Serve(f)
  \/ \E p \in People : Enable(p) \/ Disable(p) \/ Refetch(p) \/ Reload(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* React and React DOM always come from the same build.
OneReact == Loaded => react = dom
\* No module fails to load: every failure state boots production React.
NothingMissing == react # "missing" /\ dom # "missing"
\* Development React runs only when the shell chose the development map.
DevOnlyWhenChosen == Running = "dev" => map = "dev"
\* A document no service worker controls runs production React.
NoWorkerRunsProduction == ~controlled => Running = "prod"
\* The failure states run production React.
FailuresRunProduction == State \in {"absent", "mismatch"} => Running = "prod"
====
