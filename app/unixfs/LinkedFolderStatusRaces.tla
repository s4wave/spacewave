---- MODULE LinkedFolderStatusRaces ----
\* LinkedFolderStatus with every writer at once: the switch's write and
\* another session's, the upload queue's holds, a rejected root, and the
\* owner's own copy, in a cloud-backed Space and a local one.
EXTENDS LinkedFolderStatus
====
