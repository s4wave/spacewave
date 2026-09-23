# Release notes

## 0.1.0-rc.1

The first typed sync release candidate provides shared Standard Schema contracts, live ordered collections, named atomic server mutations, scoped authentication and authorization, durable SQLite storage, retained acceptance receipts, reconnect recovery, and optional React bindings.

The package ships ESM client, server, and React exports with public TypeScript declarations and a compiled engine worker. Node 24.15 to 24.x is required for the server. Complete vanilla TypeScript and React task boards use the same contract and storage.

Subscriptions share a bounded producer for identical scope, collection, and prefix queries. Each subscriber retains its own authorization checks and cancellation lifetime. Query limits fail the affected subscription while independent queries continue.

This RC supports online authoritative operation. Offline replicas, optimistic writes, receipt pruning, and SQL query APIs are outside this release. The schema version is an application compatibility contract; changing it requires an explicit maintenance migration. Receipts and World history increase storage over the dataset lifetime. See the README for measured workload limits.
