---
title: Drive and Files
section: files
order: 1
summary: Use Drive for normal file work and know when the underlying UnixFS object matters.
---

Drive is the user-facing file surface. Use it for folders, uploads, downloads, and ordinary file browsing.

Inside a Space, Drive data is stored in an object backed by UnixFS-style file operations. Most users should stay in the Drive viewer. The underlying object matters when you use the CLI, inspect Space contents, or build a plugin that reads files directly.

## Common Tasks

To add files, open the Drive Space and use upload or drag-and-drop. Small files should appear immediately. Larger files depend on browser storage, desktop runtime, and the storage provider behind the session.

To organize files, create folders inside Drive. Folder paths are part of the object data, not separate Spaces.

To move a file between devices, link the account or move the session first. Copying browser profile data manually is not a supported backup.

## CLI Shape

The CLI exposes file commands under `spacewave fs`. It addresses files by a URI that includes the session index, Space, object key, and file path:

```bash
spacewave fs ls /u/1/so/<space-id>/-/<object-key>/-/
spacewave fs write /u/1/so/<space-id>/-/<object-key>/-/notes.txt --from notes.txt
spacewave fs cat /u/1/so/<space-id>/-/<object-key>/-/notes.txt
```

Start with the app UI unless you already know the Space ID and object key. The CLI is best for repeatable scripts and inspection.
