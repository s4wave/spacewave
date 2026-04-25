import React from 'react'
import {
  LuBookOpen,
  LuBox,
  LuGitBranch,
  LuHammer,
  LuHardDrive,
  LuLayoutGrid,
  LuLink,
  LuLogIn,
  LuMessageSquare,
  LuMonitor,
  LuNotebookPen,
  LuPenLine,
  LuUser,
} from 'react-icons/lu'
import { isExperimentalCreatorVisible } from '../creator-visibility.js'

// QuickstartOption is a type that represents a quickstart option for an app with preset space contents
export interface QuickstartOption {
  id: string
  name: string
  description: string
  category: string
  icon: React.ComponentType<{ className?: string }>
  path?: string
  hidden?: boolean
  // experimental marks options only shown in dev builds (not release).
  experimental?: boolean
}

export const QUICKSTART_OPTIONS = [
  {
    id: 'account',
    name: 'Sign in or create account',
    description: 'Access your account or create a new one',
    category: 'account',
    icon: LuLogIn,
    path: '/login',
  },
  {
    id: 'pair',
    name: 'Enter a device pairing code',
    description: 'Link to an existing device via pairing code',
    category: 'account',
    icon: LuLink,
    path: '/pair',
  },
  {
    id: 'local',
    name: 'Continue without account',
    description: 'Start with a local session',
    category: 'account',
    icon: LuUser,
    hidden: true,
  },
  {
    id: 'space',
    name: 'Create an Empty Space',
    description: 'Start with a blank space',
    category: 'storage',
    icon: LuBox,
  },
  {
    id: 'drive',
    name: 'Create a Drive',
    description: 'File browser with folders, uploads, and downloads',
    category: 'storage',
    icon: LuHardDrive,
  },
  {
    id: 'git',
    name: 'Create/clone a Git Repository',
    description: 'Start fresh or clone an existing Git repository',
    category: 'storage',
    icon: LuGitBranch,
  },
  {
    id: 'notebook',
    name: 'Create a Notebook',
    description: 'Markdown notes with folders, tags, and sync',
    category: 'storage',
    icon: LuNotebookPen,
    experimental: true,
  },
  {
    id: 'canvas',
    name: 'Create a Canvas',
    description: 'Visual workspace with objects on a canvas',
    category: 'storage',
    icon: LuLayoutGrid,
  },
  {
    id: 'chat',
    name: 'Start a Chat',
    description: 'Create a space with a chat channel',
    category: 'social',
    icon: LuMessageSquare,
    experimental: true,
  },
  {
    id: 'docs',
    name: 'Create Documentation',
    description: 'Markdown documentation site',
    category: 'content',
    icon: LuBookOpen,
    experimental: true,
  },
  {
    id: 'blog',
    name: 'Create a Blog',
    description: 'Date-based markdown blog',
    category: 'content',
    icon: LuPenLine,
    experimental: true,
  },
  {
    id: 'v86',
    name: 'Create a V86 VM',
    description: 'x86 virtual machine in the browser',
    category: 'compute',
    icon: LuMonitor,
    experimental: true,
  },
  {
    id: 'forge',
    name: 'Create Forge Dashboard',
    description: 'Task orchestration dashboard',
    category: 'tools',
    icon: LuHammer,
    experimental: true,
  },
] as const satisfies readonly QuickstartOption[]

// isQuickstartOptionVisible returns true when a quickstart should be shown in
// the running app.
export function isQuickstartOptionVisible(
  option: QuickstartOption,
  isDev = !!import.meta.env?.DEV,
): boolean {
  return (
    !(option.hidden ?? false) &&
    isExperimentalCreatorVisible(option.experimental, isDev)
  )
}

// isQuickstartOptionPublic returns true when a quickstart should get a
// prerendered /quickstart/{id} page.
export function isQuickstartOptionPublic(
  option: QuickstartOption,
  isDev = !!import.meta.env?.DEV,
): boolean {
  return !option.path && isQuickstartOptionVisible(option, isDev)
}

// getVisibleQuickstartOptions returns the in-app quickstart inventory for the
// given build mode.
export function getVisibleQuickstartOptions(
  isDev = !!import.meta.env?.DEV,
): QuickstartOption[] {
  return QUICKSTART_OPTIONS.filter((option) =>
    isQuickstartOptionVisible(option, isDev),
  )
}

// getPublicQuickstartOptions returns the public prerender quickstarts for the
// given build mode.
export function getPublicQuickstartOptions(
  isDev = !!import.meta.env?.DEV,
): QuickstartOption[] {
  return QUICKSTART_OPTIONS.filter((option) =>
    isQuickstartOptionPublic(option, isDev),
  )
}

// VISIBLE_QUICKSTART_OPTIONS filters out hidden and experimental options for
// display in the running app.
export const VISIBLE_QUICKSTART_OPTIONS = getVisibleQuickstartOptions()

// PUBLIC_QUICKSTART_OPTIONS is the release-visible quickstart inventory that
// receives prerendered /quickstart/{id} pages.
export const PUBLIC_QUICKSTART_OPTIONS = getPublicQuickstartOptions()

export type QuickstartId = (typeof QUICKSTART_OPTIONS)[number]['id']

// QuickstartCreateId has quickstart IDs that create a new account as part of the option.
// Excludes the navigation-only account and pairing entries.
export type QuickstartCreateId = Exclude<QuickstartId, 'account' | 'pair'>

// QuickstartSpaceCreateId has quickstart IDs that create a new space (excludes 'local' which only creates a session).
export type QuickstartSpaceCreateId = Exclude<QuickstartCreateId, 'local'>

export function getQuickstartOption(id: QuickstartId): QuickstartOption {
  const option = QUICKSTART_OPTIONS.find((opt) => opt.id === id)
  if (!option) throw new Error(`Unknown quickstart ID: ${id}`)
  return option
}

export function getQuickstartPath(item: QuickstartOption): string {
  return item.path || `/quickstart/${item.id}`
}

// isQuickstartId checks if the given ID is a known quickstart option ID.
export function isQuickstartId(id: string): id is QuickstartId {
  return QUICKSTART_OPTIONS.some((opt) => opt.id === id)
}

// isQuickstartCreateId checks if the given ID is a quickstart create ID.
export function isQuickstartCreateId(id: string): id is QuickstartCreateId {
  return (
    id !== 'account' &&
    id !== 'pair' &&
    QUICKSTART_OPTIONS.some((opt) => opt.id === id)
  )
}
