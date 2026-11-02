import { useCallback, useState, type ReactNode } from 'react'
import {
  LuBookOpen,
  LuFolderPlus,
  LuHardDrive,
  LuUpload,
  LuUserPlus,
  LuX,
} from '@s4wave/web/images/icons.js'

import { useInvokeCommand } from '@s4wave/web/command/CommandContext.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'

import { usePath } from '@s4wave/web/router/router.js'
import {
  joinUnixFSDisplayPath,
  normalizeUnixFSLookupPath,
} from '@s4wave/sdk/unixfs/path.js'
import { UNIXFS_OBJECT_KEY } from '@s4wave/core/space/world/ops/init-unixfs.js'
import { DRIVE_STARTER_GUIDE_NAME } from '@s4wave/app/quickstart/drive-starter-guide.js'

import { useIntroWizardPresentation } from '../wizard/intro-context.js'
import {
  UnixFSBrowser,
  type UnixFSBrowserDirectoryHeaderProps,
} from './UnixFSBrowser.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4.5)`,
    height: `calc(${themeLayout['--spacing']} * 4.5)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

// DriveViewer owns Drive-specific first-run guidance while delegating all file
// operations to the generic UnixFS browser.
export function DriveViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const routerPath = usePath()
  const unixfsId = getObjectKey(objectInfo)
  const unixfsInfo =
    objectInfo?.info?.case === 'unixfsObjectInfo' ? objectInfo.info.value : null
  const basePath = unixfsInfo?.path || '/'
  const currentPath = joinUnixFSDisplayPath(basePath, routerPath || '/')
  const isDriveRoot = unixfsId === UNIXFS_OBJECT_KEY

  return (
    <UnixFSBrowser
      unixfsId={unixfsId}
      basePath={basePath}
      currentPath={currentPath}
      mimeTypeOverride={unixfsInfo?.mimeType}
      worldState={worldState}
      directoryHeader={isDriveRoot ? DriveGettingStartedHeader : undefined}
    />
  )
}

function DriveGettingStartedHeader(props: UnixFSBrowserDirectoryHeaderProps) {
  const { currentPath, entries, onNewFolder, onOpen, onUploadFiles } = props
  const [dismissed, setDismissed] = useState(false)
  // The intro wizard draws its own first-run callouts over this Drive; standing
  // the welcome header down keeps one first-run guidance surface per state
  // instead of doubling the wizard's "Add files" callout with these actions.
  const introPresenting = useIntroWizardPresentation()
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const canShareSpace = spaceCtx?.spaceSharingState?.canManage ?? false
  const invokeCommand = useInvokeCommand()

  const guideEntry =
    entries.find((entry) => entry.name === DRIVE_STARTER_GUIDE_NAME) ?? null
  const hasUserContent = entries.some(
    (entry) => entry.name && entry.name !== DRIVE_STARTER_GUIDE_NAME,
  )
  const isRoot = normalizeUnixFSLookupPath(currentPath) === ''

  const dismissThen = useCallback((action: () => void) => {
    setDismissed(true)
    action()
  }, [])
  const handleOpenGuide = useCallback(() => {
    if (!guideEntry) return
    dismissThen(() => onOpen([guideEntry]))
  }, [dismissThen, guideEntry, onOpen])
  const handleShareSpace = useCallback(() => {
    if (!canShareSpace) return
    dismissThen(() => invokeCommand('spacewave.share-space'))
  }, [canShareSpace, dismissThen, invokeCommand])

  if (introPresenting || dismissed || !isRoot || !guideEntry || hasUserContent)
    return null

  return (
    <section
      data-testid="drive-welcome"
      className="border-foreground/8 bg-background/30 very-short:max-h-1/2 very-short:overflow-y-auto border-b p-3"
    >
      <div className="flex flex-col gap-3">
        <div className="flex items-start gap-3">
          <div className="bg-brand/10 flex size-9 shrink-0 items-center justify-center rounded-md">
            <LuHardDrive sx={stylexStyles.rule1} />
          </div>
          <div className="min-w-0 flex-1">
            <h2 className="text-foreground text-sm font-semibold tracking-tight select-none">
              Welcome to your Drive
            </h2>
            <p className="text-foreground-alt/60 mt-1 text-xs leading-relaxed">
              Add files, organize folders, invite people, or open the starter
              guide.
            </p>
          </div>
          <button
            type="button"
            aria-label="Dismiss getting started"
            className="text-foreground-alt hover:text-foreground flex size-6 shrink-0 items-center justify-center rounded-md transition-colors [@media(pointer:coarse)]:size-11"
            onClick={() => setDismissed(true)}
          >
            <LuX sx={stylexStyles.rule2} />
          </button>
        </div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-4">
          <DriveGettingStartedAction
            testId="drive-upload-cta"
            icon={<LuUpload sx={stylexStyles.rule3} />}
            label="Upload files"
            description="Add content"
            onClick={() => dismissThen(onUploadFiles)}
          />
          <DriveGettingStartedAction
            testId="drive-new-folder-cta"
            icon={<LuFolderPlus sx={stylexStyles.rule3} />}
            label="New folder"
            description="Create structure"
            onClick={() => dismissThen(onNewFolder)}
          />
          <DriveGettingStartedAction
            testId="drive-open-guide-cta"
            icon={<LuBookOpen sx={stylexStyles.rule3} />}
            label="Open guide"
            description="Read getting-started.md"
            onClick={handleOpenGuide}
          />
          {canShareSpace && (
            <DriveGettingStartedAction
              testId="drive-invite-cta"
              icon={<LuUserPlus sx={stylexStyles.rule3} />}
              label="Invite people"
              description="Share this Space"
              onClick={handleShareSpace}
            />
          )}
        </div>
      </div>
    </section>
  )
}

interface DriveGettingStartedActionProps {
  description: string
  icon: ReactNode
  label: string
  onClick: () => void
  testId: string
}

function DriveGettingStartedAction({
  description,
  icon,
  label,
  onClick,
  testId,
}: DriveGettingStartedActionProps) {
  return (
    <button
      type="button"
      aria-label={label}
      data-testid={testId}
      onClick={onClick}
      sx={[[stylexStyles2.rule1, groupMarker]]}
    >
      <span className="bg-brand/10 group-hover:bg-brand/15 flex size-8 shrink-0 items-center justify-center rounded-md transition-colors">
        {icon}
      </span>
      <span className="flex min-w-0 flex-col">
        <span className="text-foreground text-xs font-medium select-none">
          {label}
        </span>
        <span className="text-foreground-alt/55 micro-label leading-snug select-none">
          {description}
        </span>
      </span>
    </button>
  )
}
