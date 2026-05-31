import {
  LuBot,
  LuBox,
  LuGitBranch,
  LuHammer,
  LuHardDrive,
  LuLayoutGrid,
  LuMessageSquare,
  LuMonitor,
  LuNotebookPen,
  LuPenLine,
} from 'react-icons/lu'

import type { QuickstartRegistration } from '@s4wave/sdk/quickstart/registry/registry.pb.js'

import { isQuickstartOptionVisible, type QuickstartOption } from './options.js'

const dynamicIconMap = {
  bot: LuBot,
  box: LuBox,
  drive: LuHardDrive,
  git: LuGitBranch,
  hammer: LuHammer,
  layout: LuLayoutGrid,
  message: LuMessageSquare,
  monitor: LuMonitor,
  notebook: LuNotebookPen,
  pen: LuPenLine,
} as const

// getDynamicQuickstartIcon returns the icon component for a dynamic Quickstart.
export function getDynamicQuickstartIcon(
  iconName?: string,
): QuickstartOption['icon'] {
  if (!iconName) return LuBox
  return dynamicIconMap[iconName as keyof typeof dynamicIconMap] ?? LuBox
}

// dynamicQuickstartRegistrationToOption converts a registry entry into app metadata.
export function dynamicQuickstartRegistrationToOption(
  reg: QuickstartRegistration,
  experimentalCreatorsEnabled = !!import.meta.env?.DEV,
): QuickstartOption | null {
  const id = reg.quickstartId ?? ''
  const name = reg.name ?? ''
  const description = reg.description ?? ''
  const category = reg.category ?? ''
  const pluginId = reg.pluginId ?? ''
  if (!id || !name || !description || !category || !pluginId) return null

  const option: QuickstartOption = {
    id,
    name,
    description,
    category,
    icon: getDynamicQuickstartIcon(reg.iconName),
    hidden: reg.hidden ?? false,
    experimental: reg.experimental ?? false,
    dynamic: true,
    pluginId,
    spaceName: reg.spaceName || name,
    requiredPluginIds: reg.requiredPluginIds ?? [],
  }
  if (!isQuickstartOptionVisible(option, experimentalCreatorsEnabled)) {
    return null
  }
  return option
}

// mergeQuickstartOptions appends dynamic app-only Quickstarts after static entries.
export function mergeQuickstartOptions(
  staticOptions: QuickstartOption[],
  dynamicRegistrations: QuickstartRegistration[],
  experimentalCreatorsEnabled = !!import.meta.env?.DEV,
): QuickstartOption[] {
  const seen = new Set(staticOptions.map((option) => option.id))
  const dynamicOptions: QuickstartOption[] = []
  for (const reg of dynamicRegistrations) {
    const option = dynamicQuickstartRegistrationToOption(
      reg,
      experimentalCreatorsEnabled,
    )
    if (!option || seen.has(option.id)) continue
    seen.add(option.id)
    dynamicOptions.push(option)
  }
  return [...staticOptions, ...dynamicOptions]
}
