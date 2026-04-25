import { describe, expect, it } from 'vitest'

import { GraphLinkPredicatePolicy } from './graphLinkPolicy.js'

describe('GraphLinkPredicatePolicy', () => {
  it('hides protected type predicates from the graph-link view', () => {
    const policy = new GraphLinkPredicatePolicy()

    expect(policy.classify({ predicate: '<type>' })).toEqual({
      viewable: false,
      hideable: false,
      userRemovable: false,
      protected: true,
      ownerManaged: true,
    })
  })

  it('keeps owner-managed predicates visible but not user-removable', () => {
    const policy = new GraphLinkPredicatePolicy()

    expect(policy.classify({ predicate: '<parent>' })).toEqual({
      viewable: true,
      hideable: true,
      userRemovable: false,
      protected: false,
      ownerManaged: true,
    })
  })

  it('allows generic predicates to be hidden and removed by the user', () => {
    const policy = new GraphLinkPredicatePolicy()

    expect(policy.classify({ predicate: '<relatedTo>' })).toEqual({
      viewable: true,
      hideable: true,
      userRemovable: true,
      protected: false,
      ownerManaged: false,
    })
  })
})
