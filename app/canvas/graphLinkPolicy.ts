import type { HiddenGraphLinkData } from './types.js'

export interface GraphLinkPolicyResult {
  viewable: boolean
  hideable: boolean
  userRemovable: boolean
  protected: boolean
  ownerManaged: boolean
}

const hiddenTypePredicate = '<type>'
const protectedPredicates = new Set([hiddenTypePredicate])
const ownerManagedPredicates = new Set(['<parent>', '<workdir>'])

export class GraphLinkPredicatePolicy {
  classify(
    link: Pick<HiddenGraphLinkData, 'predicate'>,
  ): GraphLinkPolicyResult {
    if (protectedPredicates.has(link.predicate)) {
      return {
        viewable: false,
        hideable: false,
        userRemovable: false,
        protected: true,
        ownerManaged: true,
      }
    }

    if (ownerManagedPredicates.has(link.predicate)) {
      return {
        viewable: true,
        hideable: true,
        userRemovable: false,
        protected: false,
        ownerManaged: true,
      }
    }

    return {
      viewable: true,
      hideable: true,
      userRemovable: true,
      protected: false,
      ownerManaged: false,
    }
  }
}

export const graphLinkPredicatePolicy = new GraphLinkPredicatePolicy()
