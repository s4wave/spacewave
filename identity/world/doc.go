// Package identity_world stores identity records as World objects.
//
// An Entity lives at "e/<domain-id>/<entity-id>" (type identity/entity), a
// Keypair at "kp/<base58 peer id>" (type identity/keypair), and a domain's
// info at "d/<domain-id>" (type identity/domain). Quads link an entity to its
// domain info and any object to the keypairs it references. The update
// operations write each record through the World's operation path, so every
// change is an ordinary World transaction that the World's Authority admits.
package identity_world
