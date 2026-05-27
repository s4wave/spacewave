import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuCopy, LuExternalLink } from 'react-icons/lu'

import type { AddTabRequest } from '@s4wave/sdk/layout/layout.pb.js'
import {
  createObjectLayoutAddTabRequest,
  createObjectLayoutObjectInfo,
} from '@s4wave/sdk/layout/world/object-layout.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import {
  getObjectTypeIcon,
  getObjectTypeLabel,
} from '@s4wave/web/space/object-tree.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import type { ObjectInfo } from './object.pb.js'
import { useTabContext } from './TabContext.js'

export interface ObjectLinkProps {
  objectKey?: string
  objectType?: string
  label?: string
  kind?: string
  status?: string
  componentID?: string
  path?: string
  className?: string
  compact?: boolean
}

export interface ObjectLinkTarget {
  objectKey: string
  objectType?: string
  label?: string
  componentID?: string
  path?: string
}

export function ObjectLink({
  objectKey,
  objectType,
  label,
  kind,
  status,
  componentID,
  path,
  className,
  compact = false,
}: ObjectLinkProps) {
  const spaceContainer = SpaceContainerContext.useContextSafe()
  const tabContext = useTabContext()
  const [copied, setCopied] = useState(false)
  const targetKey = objectKey?.trim() ?? ''
  const missing = targetKey === ''
  const displayLabel = useMemo(
    () => objectLinkDisplayLabel({ objectKey: targetKey, label }),
    [targetKey, label],
  )
  const kindLabel = kind || (objectType ? getObjectTypeLabel(objectType) : '')
  const canOpenInNewTab = Boolean(
    targetKey && spaceContainer?.buildObjectUrls([targetKey])[0],
  )

  const handlePrimary = useCallback(() => {
    if (!targetKey) return
    if (tabContext?.isObjectLayout) {
      void tabContext
        .addTab(
          createObjectLinkAddTabRequest({
            objectKey: targetKey,
            objectType,
            label,
            componentID,
            path,
            currentTabId: tabContext.tabId,
          }),
        )
        .catch((err) => console.warn('ObjectLink: failed to add tab:', err))
      return
    }
    spaceContainer?.navigateToObjects([targetKey])
  }, [
    componentID,
    label,
    objectType,
    path,
    spaceContainer,
    tabContext,
    targetKey,
  ])

  const handleCopy = useCallback(() => {
    if (!targetKey) return
    void navigator.clipboard
      .writeText(targetKey)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      })
      .catch((err) =>
        console.warn('ObjectLink: failed to copy object key:', err),
      )
  }, [targetKey])

  const handleOpenInNewTab = useCallback(() => {
    if (!targetKey) return
    const url = spaceContainer?.buildObjectUrls([targetKey])[0]
    if (url) {
      window.open(url, '_blank', 'noopener,noreferrer')
      return
    }
    spaceContainer?.navigateToObjects([targetKey])
  }, [spaceContainer, targetKey])

  const accessibleLabel = displayLabel || targetKey || 'missing object ref'

  return (
    <span
      className={cn(
        'border-foreground/8 bg-foreground/5 text-foreground-alt/80 inline-flex max-w-full items-center rounded-md border align-middle',
        missing && 'border-warning/20 bg-warning/5 text-warning',
        compact ? 'min-h-6' : 'min-h-7',
        className,
      )}
      data-object-link={targetKey || undefined}
      data-missing-object-link={missing ? 'true' : undefined}
    >
      <button
        type="button"
        disabled={missing}
        aria-label={'Open ' + accessibleLabel}
        onClick={handlePrimary}
        className={cn(
          'inline-flex min-w-0 flex-1 items-center gap-1.5 rounded-l-md text-left transition-colors',
          compact ? 'px-1.5 py-0.5' : 'px-2 py-1',
          missing
            ? 'cursor-not-allowed'
            : 'hover:bg-foreground/5 text-foreground',
        )}
      >
        <span className="text-foreground-alt/60 shrink-0">
          {objectType ? getObjectTypeIcon(objectType) : getObjectTypeIcon('')}
        </span>
        <span className="min-w-0">
          <span className="block truncate font-mono text-[0.6rem] leading-4">
            {missing ? displayLabel || 'Missing ref' : displayLabel}
          </span>
          {(kindLabel || status) && !compact ? (
            <span className="text-foreground-alt/50 block truncate text-[0.55rem] leading-3">
              {[kindLabel, status].filter(Boolean).join(' / ')}
            </span>
          ) : null}
        </span>
      </button>
      {missing ? null : (
        <>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={'Copy ' + accessibleLabel}
                onClick={handleCopy}
                className={cn(
                  'border-foreground/8 hover:bg-foreground/5 inline-flex shrink-0 items-center justify-center border-l transition-colors',
                  compact ? 'h-6 w-6' : 'h-7 w-7',
                )}
              >
                {copied ? (
                  <LuCheck className="text-success h-3.5 w-3.5" />
                ) : (
                  <LuCopy className="h-3.5 w-3.5" />
                )}
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              {copied ? 'Copied' : 'Copy object key'}
            </TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={'Open ' + accessibleLabel + ' in new tab'}
                disabled={!canOpenInNewTab && !spaceContainer}
                onClick={handleOpenInNewTab}
                className={cn(
                  'border-foreground/8 hover:bg-foreground/5 disabled:text-foreground-alt/30 inline-flex shrink-0 items-center justify-center rounded-r-md border-l transition-colors disabled:cursor-not-allowed disabled:hover:bg-transparent',
                  compact ? 'h-6 w-6' : 'h-7 w-7',
                )}
              >
                <LuExternalLink className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom">Open in new tab</TooltipContent>
          </Tooltip>
        </>
      )}
    </span>
  )
}

export function createObjectLinkAddTabRequest(
  target: ObjectLinkTarget & { currentTabId?: string },
): AddTabRequest {
  return createObjectLayoutAddTabRequest({
    afterTabId: target.currentTabId,
    select: true,
    id: objectLinkTabId(target),
    name: objectLinkDisplayLabel(target),
    objectKey: target.objectKey,
    objectType: target.objectType,
    componentID: target.componentID,
    path: target.path,
    helpText: target.objectKey,
    enableClose: true,
  })
}

export function createObjectLinkObjectInfo(
  target: ObjectLinkTarget,
): ObjectInfo {
  return createObjectLayoutObjectInfo(target)
}

export function objectLinkTabId(target: ObjectLinkTarget): string {
  const raw = [
    target.objectKey,
    target.objectType,
    target.componentID,
    target.path,
  ]
    .filter(Boolean)
    .join('|')
  const slug =
    raw
      .toLowerCase()
      .replace(/[^a-z0-9]+/gu, '-')
      .replace(/^-+|-+$/gu, '')
      .slice(0, 80) || 'object'
  return 'object-link-' + slug + '-' + stableHash(raw)
}

export function objectLinkDisplayLabel(target: {
  objectKey?: string
  label?: string
}): string {
  const label = target.label?.trim()
  if (label) return label
  const key = target.objectKey?.trim()
  if (!key) return 'Missing ref'
  const parts = key.split('/').filter(Boolean)
  return parts.at(-1) ?? key
}

function stableHash(value: string): string {
  const hash = Array.from(value).reduce(
    (acc, char) => Math.imul(acc ^ char.charCodeAt(0), 16777619),
    2166136261,
  )
  return (hash >>> 0).toString(36)
}
