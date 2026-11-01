// PublicationState holds the superblock generation and reader pin.
type PublicationState = { durableSuper: number; newest: number; pin: number; released: number[] }

// title names the publication and reuse boundary.
export const title = 'Publish a durable manifest before reusing old chunks'

// state groups jobs by the boundary a crash or reader can observe.
export const state = (v: PublicationState) =>
  v.released.length === 2 ? 'Old chunks reused' :
  v.released.length > 0 ? 'Some old chunks reused' :
  `${v.newest > v.durableSuper ? 'Superblock awaiting sync' : v.durableSuper > 0 ? 'Manifest durable' : 'Original manifest'}${v.pin === 0 ? ', reader holds original' : ', reader ended'}`

// step hides manifest IO until it changes a durable or reuse boundary.
export const step = (action: string, from: PublicationState, to: PublicationState) =>
  state(from) === state(to) ? null : action
