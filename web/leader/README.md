# Leader

This package implements leader election in a web browser context.

It uses BroadcastChannel and IndexedDB to elect a leader by ID.

This is used to de-conflict WebWorker in the Bldr runtime.
