# Payload page publication

`PayloadPages.tla` models the physical closure needed by three producers:
a staged payload that reuses replay metadata, a new atomic candidate with
kept children, and a direct reference to an existing kept page. Both atomic
and fallback publication preserve captured bytes across a restart.

The normal configuration keeps every advertised payload physically readable.
Each configuration here removes one required carry boundary. TLC must reject
all three with `PublishedReadable`: staging, atomic publication and fallback
write-before-advertisement.

The graph is a bounded four-node abstraction of these ownership boundaries.
It does not prove the collector, arbitrary graphs or publication liveness.
