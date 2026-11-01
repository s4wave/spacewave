---------------------------- MODULE StylexOrder ----------------------------
EXTENDS Naturals, Integers, Sequences, FiniteSets

CONSTANTS StablePriorities, SortedOrder

Sheets == {"shared", "plugin"}
Priority == [shared |-> 4000, plugin |-> 1000]
Layer(sheet) == IF StablePriorities THEN Priority[sheet] ELSE 1

VARIABLES loaded, arrivals, known, order
vars == <<loaded, arrivals, known, order>>

Init == /\ loaded = {}
        /\ arrivals = <<>>
        /\ known = {}
        /\ order = <<>>

Ordered(priorities) ==
    IF priorities = {} THEN <<>>
    ELSE IF priorities = {1} THEN <<1>>
    ELSE IF priorities = {1000} THEN <<1000>>
    ELSE IF priorities = {4000} THEN <<4000>>
    ELSE <<1000, 4000>>

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
          /\ known \subseteq {1, 1000, 4000}
          /\ arrivals \in Seq(Sheets)
          /\ order \in Seq({1, 1000, 4000})

\* WebView reveals after stylesheet load handling, which registers the order
\* in the same browser task. No paint observes an intermediate ordering.
PropertySpecificity == loaded = Sheets => SharedWins

Spec == Init /\ [][Next]_vars
=============================================================================
