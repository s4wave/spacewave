import { useCallback, useState, type ReactNode } from 'react'
import {
  LuBookOpen,
  LuFolderPlus,
  LuHardDrive,
  LuUpload,
  LuUserPlus,
  LuX,
} from 'react-icons/lu'

import { useInvokeCommand } from '@s4wave/web/command/CommandContext.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { cn } from '@s4wave/web/style/utils.js'
import { usePath } from '@s4wave/web/router/router.js'
import {
  joinUnixFSDisplayPath,
  normalizeUnixFSLookupPath,
} from '@s4wave/sdk/unixfs/path.js'
import { UNIXFS_OBJECT_KEY } from '@s4wave/core/space/world/ops/init-unixfs.js'

import {
  UnixFSBrowser,
  type UnixFSBrowserDirectoryHeaderProps,
} from './UnixFSBrowser.js'

const DRIVE_STARTER_GUIDE_NAME = 'getting-started.md'

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

  if (dismissed || !isRoot || !guideEntry || hasUserContent) return null

  return (
    <section
      data-testid="drive-welcome"
      className="border-foreground/8 bg-background/30 border-b p-3"
    >
      <div className="flex flex-col gap-3">
        <div className="flex items-start gap-3">
          <div className="bg-brand/10 flex size-9 shrink-0 items-center justify-center rounded-md">
            <LuHardDrive className="text-brand size-4.5" />
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
            className="text-foreground-alt hover:text-foreground rounded-md p-1 transition-colors"
            onClick={() => setDismissed(true)}
          >
            <LuX className="size-4" />
          </button>
        </div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-4">
          <DriveGettingStartedAction
            testId="drive-upload-cta"
            icon={<LuUpload className="text-foreground-alt size-4" />}
            label="Upload files"
            description="Add content"
            onClick={() => dismissThen(onUploadFiles)}
          />
          <DriveGettingStartedAction
            testId="drive-new-folder-cta"
            icon={<LuFolderPlus className="text-foreground-alt size-4" />}
            label="New folder"
            description="Create structure"
            onClick={() => dismissThen(onNewFolder)}
          />
          <DriveGettingStartedAction
            testId="drive-open-guide-cta"
            icon={<LuBookOpen className="text-foreground-alt size-4" />}
            label="Open guide"
            description="Read getting-started.md"
            onClick={handleOpenGuide}
          />
          {canShareSpace && (
            <DriveGettingStartedAction
              testId="drive-invite-cta"
              icon={<LuUserPlus className="text-foreground-alt size-4" />}
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
      className={cn(
        'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50',
        'group flex min-h-18 min-w-0 items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
      )}
    >
      <span className="bg-foreground/5 group-hover:bg-foreground/8 flex size-8 shrink-0 items-center justify-center rounded-md transition-colors">
        {icon}
      </span>
      <span className="flex min-w-0 flex-col">
        <span className="text-foreground text-xs font-medium select-none">
          {label}
        </span>
        <span className="text-foreground-alt/55 text-[0.65rem] leading-snug select-none">
          {description}
        </span>
      </span>
    </button>
  )
}
