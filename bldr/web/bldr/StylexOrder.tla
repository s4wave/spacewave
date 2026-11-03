---------------------------- MODULE StylexOrder ----------------------------
EXTENDS Naturals, Integers, Sequences, FiniteSets

CONSTANTS StablePriorities, SortedOrder

Sheets == {"shared", "plugin", "small", "large", "duplicate"}
\* Integers represent the lexicographic compiler bucket order, not numeric
\* priorities alone. Small and large share a compiler priority; duplicate is
\* independently emitted CSS for the exact same smaller-breakpoint bucket.
Priority == [shared |-> 4000, plugin |-> 1000,
             small |-> 430032, large |-> 430042, duplicate |-> 430032]
Layer(sheet) == IF StablePriorities THEN Priority[sheet] ELSE 1

VARIABLES loaded, arrivals, known, order
vars == <<loaded, arrivals, known, order>>

Init == /\ loaded = {}
        /\ arrivals = <<>>
        /\ known = {}
        /\ order = <<>>

RECURSIVE Ordered(_)
Ordered(priorities) ==
    IF priorities = {} THEN <<>>
    ELSE LET first == CHOOSE value \in priorities :
                          \A other \in priorities : value <= other
         IN <<first>> \o Ordered(priorities \ {first})

Load(sheet) ==
    /\ sheet \in Sheets \ loaded
    /\ loaded' = loaded \cup {sheet}
    /\ arrivals' = Append(arrivals, sheet)
    /\ known' = known \cup {Layer(sheet)}
    /\ order' = IF SortedOrder THEN Ordered(known')
                ELSE IF Layer(sheet) \in known THEN order
                     ELSE Append(order, Layer(sheet))

Next == \/ \E sheet \in Sheets : Load(sheet)
        \/ /\ loaded = Sheets
           /\ UNCHANGED vars

Index(layer) == CHOOSE index \in 1..Len(order) : order[index] = layer
SharedWins ==
    IF Layer("shared") = Layer("plugin")
    THEN arrivals[Len(arrivals)] = "shared"
    ELSE Index(Layer("shared")) > Index(Layer("plugin"))

TypeOK == /\ loaded \subseteq Sheets
          /\ known \subseteq {1, 1000, 4000, 430032, 430042}
          /\ arrivals \in Seq(Sheets)
          /\ order \in Seq({1, 1000, 4000, 430032, 430042})

\* WebView reveals after stylesheet load handling, which registers the order
\* in the same browser task. No paint observes an intermediate ordering.
LargerBreakpointWins ==
    IF Layer("large") = Layer("small") THEN FALSE
    ELSE Index(Layer("large")) > Index(Layer("small"))

PropertySpecificity == loaded = Sheets => SharedWins /\ LargerBreakpointWins

Spec == Init /\ [][Next]_vars
=============================================================================
