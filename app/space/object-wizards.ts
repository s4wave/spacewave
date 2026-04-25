import type { ObjectWizard } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { isExperimentalCreatorVisible } from '../creator-visibility.js'
import { lookupCreateOpBuilder } from './create-op-builders.js'

function getWizardScore(wizard: ObjectWizard): number {
  let score = 0
  if (wizard.displayName) score += 1
  if (wizard.category) score += 1
  if (wizard.iconName) score += 1
  if (wizard.createOpId) score += 2
  if (wizard.keyPrefix) score += 2
  if (wizard.persistent && wizard.wizardTypeId) score += 4
  return score
}

function isWizardCreatable(wizard: ObjectWizard): boolean {
  if (!wizard.typeId || !wizard.displayName) return false
  if (wizard.persistent && wizard.wizardTypeId) return true
  if (!wizard.createOpId || !wizard.keyPrefix) return false
  return !!lookupCreateOpBuilder(wizard.createOpId)
}

// isObjectWizardVisible returns true when the wizard should be shown for the
// given build mode.
export function isObjectWizardVisible(
  wizard: ObjectWizard,
  isDev = !!import.meta.env?.DEV,
): boolean {
  return isExperimentalCreatorVisible(wizard.experimental, isDev)
}

// normalizeObjectWizards filters malformed wizard entries and deduplicates
// them by type ID so the drawer and command palette render stable lists.
export function normalizeObjectWizards(
  wizards: ObjectWizard[],
  isDev = !!import.meta.env?.DEV,
): ObjectWizard[] {
  const deduped = new Map<string, ObjectWizard>()
  for (const wizard of wizards) {
    if (!isObjectWizardVisible(wizard, isDev)) continue
    if (!isWizardCreatable(wizard)) continue
    const typeId = wizard.typeId ?? ''
    const existing = deduped.get(typeId)
    if (!existing) {
      deduped.set(typeId, wizard)
      continue
    }
    if (getWizardScore(wizard) > getWizardScore(existing)) {
      deduped.set(typeId, wizard)
    }
  }
  return [...deduped.values()]
}
