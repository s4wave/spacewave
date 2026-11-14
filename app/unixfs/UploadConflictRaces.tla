---- MODULE UploadConflictRaces ----
\* UploadConflict with every start and every other writer: each name may
\* hold nothing, a file or a folder before the drop, and another writer may
\* leave it holding any of them while it uploads or asks.
EXTENDS UploadConflict
====
