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

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    maxHeight: { default: null, '@media (max-height: 470px)': '50%' },
    overflowY: { default: null, '@media (max-height: 470px)': 'auto' },
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule5: { minWidth: 0, flex: 1 },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule7: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule8: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule9: {
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
      '@media (min-width: 80rem)': 'repeat(4, minmax(0, 1fr))',
    },
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule11: { display: 'flex', minWidth: 0, flexDirection: 'column' },
  rule12: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule13: {
    fontSize: '.65rem',
    lineHeight: 'var(--leading-snug)',
    '--tw-leading': 'var(--leading-snug)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

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
    <section data-testid="drive-welcome" sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <div sx={stylexStyles3.rule4}>
            <LuHardDrive sx={stylexStyles.rule1} />
          </div>
          <div sx={stylexStyles3.rule5}>
            <h2 sx={stylexStyles3.rule6}>Welcome to your Drive</h2>
            <p sx={stylexStyles3.rule7}>
              Add files, organize folders, invite people, or open the starter
              guide.
            </p>
          </div>
          <button
            type="button"
            aria-label="Dismiss getting started"
            sx={stylexStyles3.rule8}
            onClick={() => setDismissed(true)}
          >
            <LuX sx={stylexStyles.rule2} />
          </button>
        </div>
        <div sx={stylexStyles3.rule9}>
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
      <span sx={stylexStyles3.rule10}>{icon}</span>
      <span sx={stylexStyles3.rule11}>
        <span sx={stylexStyles3.rule12}>{label}</span>
        <span sx={stylexStyles3.rule13}>{description}</span>
      </span>
    </button>
  )
}
