---- MODULE ObjectBrowser ----
EXTENDS Naturals
\* The Space object browser, rebuilt around types and relations. A person opens
\* on the types present in the Space, opens a type to list its objects by
\* title, opens an object to see its relations, and follows a relation to the
\* next object. Relations show as a graph or as a grouped list, both drawn from
\* one relation read, so switching between them reads nothing. Search runs next
\* to the data and its results keep the search field; the key tree stays as an
\* expert view. Another writer can add or delete objects while the screen is
\* open, and the connection to the Space can drop.
\*
\* The browser offers no writes, so a member who may only read and one who may
\* also write see the same screen; People has one member for that reason. A
\* person without access to the Space never reaches this screen.

People == {"member"}
Buttons == {"openType", "openObject", "follow", "back", "search", "openKeys", "retry", "copyKey",
            "open", "toggleRelations"}

\* view: the list or object on screen. load: the read behind it.
\* conn: the connection to the Space. target: the open object, which another
\* writer may delete. count: typed objects in the World: none, one or many.
\* rel: how an object's relations show, which a person keeps across objects.
VARIABLES view, load, conn, target, count, rel
vars == <<view, load, conn, target, count, rel>>

Views == {"types", "objects", "object", "search", "keys"}
Parent(v) == IF v = "object" THEN "objects" ELSE "types"
Reads == {"openType", "openObject", "follow", "search", "openKeys", "retry"}

\* The World resource serves a read only over a live connection; a list
\* offers an object only when it holds one, and only a present object has
\* relations to follow, a key to copy and a viewer to open. The relations
\* switch between graph and list once read.
Allowed(p, a) ==
  CASE a = "back" -> view # "types"
    [] a \in {"copyKey", "open"} -> view = "object" /\ target = "present"
    [] a = "toggleRelations" -> view = "object" /\ target = "present" /\ load = "ready"
    [] a = "retry" -> conn = "up" /\ load = "failed"
    [] a = "follow" -> conn = "up" /\ view = "object" /\ target = "present" /\ load = "ready"
    [] a \in {"openType", "openObject"} -> conn = "up" /\ count > 0
    [] OTHER -> conn = "up"

\* The actions on screen, derived from the state alone.
Shown(p) ==
  LET nav == IF view = "types" THEN {} ELSE {"back"}
      online == conn = "up"
  IN CASE load = "loading" -> nav
       [] load = "failed" -> nav \cup (IF online THEN {"retry"} ELSE {})
       [] ~online -> nav \cup (IF view = "object" /\ target = "present"
                                THEN {"copyKey", "open", "toggleRelations"} ELSE {})
       [] view = "types" -> (IF count > 0 THEN {"openType"} ELSE {}) \cup {"search", "openKeys"}
       [] view \in {"objects", "search", "keys"} -> nav \cup (IF count > 0 THEN {"openObject"} ELSE {})
                                                    \cup (IF view = "search" THEN {"search"} ELSE {})
       [] target = "deleted" -> nav
       [] OTHER -> nav \cup {"follow", "copyKey", "open", "toggleRelations"}

\* What the screen says when a person has nothing to press, or why an
\* action is missing.
Hint(p) ==
  CASE load = "loading" -> "Loading"
    [] conn = "down" -> "Offline: reconnecting"
    [] load = "failed" -> "Could not load"
    [] view = "object" /\ target = "deleted" -> "Deleted by another writer"
    [] count = 0 -> "This Space has no objects yet"
    [] OTHER -> ""

Init == view = "types" /\ load = "loading" /\ conn = "up" /\ target = "present" /\ count \in {0, 1, 2}
        /\ rel = "graph"

Next0 == UNCHANGED <<conn, count, rel>>
Go(v) == view' = v /\ load' = "loading" /\ target' = "present" /\ Next0

Press(p, a) ==
  /\ a \in Shown(p)
  /\ CASE a = "openType" -> Go("objects")
       [] a = "openObject" -> Go("object")
       [] a = "follow" -> Go("object")
       [] a = "search" -> Go("search")
       [] a = "openKeys" -> Go("keys")
       [] a = "retry" -> load' = "loading" /\ UNCHANGED <<view, target, conn, count, rel>>
       [] a = "toggleRelations" -> rel' = (IF rel = "graph" THEN "list" ELSE "graph")
                                   /\ UNCHANGED <<view, load, target, conn, count>>
       [] a = "back" -> view' = Parent(view) /\ load' = "loading" /\ target' = "present" /\ Next0
       [] a \in {"copyKey", "open"} -> UNCHANGED vars

Loaded == load = "loading" /\ conn = "up" /\ load' = "ready" /\ UNCHANGED <<view, target, conn, count, rel>>
LoadFails == load = "loading" /\ load' = "failed" /\ UNCHANGED <<view, target, conn, count, rel>>
Disconnect == conn = "up" /\ conn' = "down" /\ UNCHANGED <<view, load, target, count, rel>>
\* A reconnect reruns a read that failed while offline.
Reconnect == conn = "down" /\ conn' = "up" /\ load' = (IF load = "failed" THEN "loading" ELSE load)
             /\ UNCHANGED <<view, target, count, rel>>
OtherAdds == count < 2 /\ count' = count + 1 /\ UNCHANGED <<view, load, conn, target, rel>>
OtherDeletes ==
  /\ count > 0 /\ count' = count - 1
  /\ target' = IF view = "object" THEN "deleted" ELSE target
  /\ UNCHANGED <<view, load, conn, rel>>

Next ==
  \/ \E p \in People, a \in Buttons : Press(p, a)
  \/ Loaded \/ LoadFails \/ Disconnect \/ Reconnect \/ OtherAdds \/ OtherDeletes

TypeOK == view \in Views /\ load \in {"loading", "ready", "failed"} /\ conn \in {"up", "down"}
          /\ target \in {"present", "deleted"} /\ count \in {0, 1, 2} /\ rel \in {"graph", "list"}

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* A deleted object offers no relation to follow, no key to copy, no viewer to
\* open and no relations to switch.
NothingOnDeleted == view = "object" /\ target = "deleted" =>
  Shown("member") \cap {"follow", "copyKey", "open", "toggleRelations"} = {}
\* Offline, the screen starts no read.
NoReadOffline == conn = "down" => Shown("member") \cap Reads = {}
\* A person can always get back to the types list.
CanGoHome == view # "types" => "back" \in Shown("member")
====
