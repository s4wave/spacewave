import { WIZARD_ENDING_PREDICATE_PREFIX } from '@s4wave/sdk/world/wizard/wizard-ending.js'

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
// protectedPredicatePrefixes start predicates that carry a key, such as a
// wizard ending's, so no fixed set can list them.
const protectedPredicatePrefixes = [WIZARD_ENDING_PREDICATE_PREFIX]
const ownerManagedPredicates = new Set(['<parent>', '<workdir>'])

// GraphLinkPredicatePolicy classifies world graph links for Canvas actions.
export class GraphLinkPredicatePolicy {
  classify(
    link: Pick<HiddenGraphLinkData, 'predicate'>,
  ): GraphLinkPolicyResult {
    if (
      protectedPredicates.has(link.predicate) ||
      protectedPredicatePrefixes.some((p) => link.predicate.startsWith(p))
    ) {
      return {
        viewable: false,
        hideable: false,
        userRemovable: false,
        protected: true,
        ownerManaged: true,
      }
    }

    // TODO: Replace this TypeScript predicate table with ObjectType metadata
    // once object types declare their owner-managed graph edges.
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
