// Package volume defines the Volume: one peer's durable store of buckets,
// blocks, objects and the roots that keep blocks live.
//
// A Volume has a peer identity, and its ID is usually derived from that peer
// and the store type. It owns three contracts the layers above rely on:
//
//   - Durability. Sync is the volume's barrier: every write issued before it
//     is durable when it returns. Each backend states how a commit becomes
//     durable, such as package s4db's single-file log or package paylog's
//     index commit that publishes its segment writes.
//   - Liveness. Roots holds the named roots of the buckets and the leased
//     roots of reader pins and stages. A block stays stored while a root
//     reaches it, and the volume's collector frees only what none reaches. A
//     volume whose store cannot commit liveness with its blocks holds no
//     roots, and frees nothing by liveness.
//   - Coordination. The embedded coord.Coordinator serves the leases and
//     watches that let several writers share the volume's ObjectStore.
//
// Close releases the volume and keeps its data; Delete closes it and removes
// the backing store.
package volume
