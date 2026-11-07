---
title: Read Blocks the CDN Lacks
section: sync
order: 4
summary: Read a CDN Space first and your server's block store second, so a head newer than the last mirror still loads and a reconnect does not fail a checkout.
---

A mirror copies a World to its CDN Space in batches, so a client that learns of
a newer head from your server can ask for blocks the Space does not hold yet.
`cdn_bstore.NewChainedStore` reads each block from the CDN Space first and
from your server's block store second:

```go
rpcStore := block_rpc_client.NewBlockStore(
	block_rpc.NewSRPCBlockStoreClient(srpc.NewClient(
		stream_srpc.NewOpenStreamFunc(b, protocolID, "", serverPeer, 0),
	)),
	hashType,
	true, // read only
)
store := cdn_bstore.NewChainedStore(ctx, le, cdnStore, rpcStore)
```

Blocks the Space's packs hold come from the CDN, and only the newest blocks
cost your server a read. Open a `bucket_lookup.Cursor` over `store` at the
newer head and read the World as usual.

## Reconnects

Build the RPC client on an open function that opens each call on the current
connection, as `stream_srpc.NewOpenStreamFunc` does: while the link is down,
it waits for the next one. When a read loses its connection before the server
answers, the chained store sends it again with a growing delay, up to a second
apart, until the server answers or the read's context ends. A long checkout
rides through a reconnect instead of failing.

Only reads are sent again. A write lost with its connection may still have
applied, so its error reaches the caller. An error the server's store returns,
such as a refused read, is the answer and is not retried.
