import { iriToKey, keyToIRI } from '../graph-utils.js'
import type { IWorldState } from '../world-state.js'

// WIZARD_ENDINGS_OBJECT_KEY is the key of the one object per Space that holds
// every wizard ending as an outgoing quad. It mirrors WizardEndingsObjectKey.
export const WIZARD_ENDINGS_OBJECT_KEY = 'wizard-endings'

// WIZARD_ENDINGS_TYPE_ID is the type of the endings object, which listings
// hide. It mirrors WizardEndingsTypeID.
export const WIZARD_ENDINGS_TYPE_ID = 'space/wizard-endings'

// WIZARD_ENDING_PREDICATE_PREFIX starts the predicate IRI of every ending
// quad; the wizard's key follows it.
export const WIZARD_ENDING_PREDICATE_PREFIX = '<wizard/ended/'

// wizardEndingPredicate is the predicate IRI of the ending quad for the
// wizard at wizardKey.
export function wizardEndingPredicate(wizardKey: string): string {
  return WIZARD_ENDING_PREDICATE_PREFIX + wizardKey + '>'
}

// lookupWizardEnding returns the key of the object the wizard at wizardKey
// finished as, or undefined when the World records no ending for it: the
// wizard exists, was deleted without a Finish, or its target was deleted
// since. RecordWizardEndingOp writes the ending in the Finish transaction.
export async function lookupWizardEnding(
  world: IWorldState,
  wizardKey: string,
  signal?: AbortSignal,
): Promise<string | undefined> {
  const found = await world.lookupGraphQuads(
    keyToIRI(WIZARD_ENDINGS_OBJECT_KEY),
    wizardEndingPredicate(wizardKey),
    undefined,
    undefined,
    1,
    signal,
  )
  const target = found.quads?.[0]?.obj
  return target ? iriToKey(target) : undefined
}
