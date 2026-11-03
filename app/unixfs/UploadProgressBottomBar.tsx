import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import type { IconType } from '@s4wave/web/images/icons.js'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  LuCheck,
  LuCircleAlert,
  LuFolder,
  LuUpload,
  LuX,
} from '@s4wave/web/images/icons.js'

import { isStorageQuotaError } from '@s4wave/app/session/storage/storage-error.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import type {
  UploadEvent,
  UploadManager,
  UploadItem,
} from './useUploadManager.js'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import * as stylex from '@stylexjs/stylex'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles6 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
  },
})

const stylexStyles5 = stylex.create({
  rule1: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-green-500']} 10%, transparent)`,
    color: themeColors['--color-green-500'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    whiteSpace: 'nowrap',
  },
  rule2: { color: themeColors['--color-foreground'] },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    minHeight: 0,
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-popover'],
  },
  rule2: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: { minWidth: 0 },
  rule4: {
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
      },
    },
  },
  rule9: {
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(2, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'repeat(4, minmax(0, 1fr))',
    },
    gap: '1px',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
    backgroundColor: themeColors['--color-popover-border'],
  },
  rule10: {
    borderColor: themeColors['--color-popover-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule11: {
    height: `calc(${themeLayout['--spacing']} * 2)`,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-muted'],
  },
  rule12: {
    transition: 'width 200ms',
    width: 'var(--upload-progress-width)',
    height: '100%',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule14: { minHeight: 0, flex: 1, overflowY: 'auto' },
  rule15: {
    minWidth: 0,
    backgroundColor: themeColors['--color-popover'],
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    letterSpacing: '.12em',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule17: {
    marginTop: themeLayout['--spacing'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule18: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule19: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule20: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    minWidth: `calc(${themeLayout['--spacing']} * 40)`,
    flex: 1,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-muted'],
  },
  rule21: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule22: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderTopStyle: {
      default: themeUtilityState['--tw-border-style'],
      ':first-child': themeUtilityState['--tw-border-style'],
    },
    borderTopWidth: { default: '1px', ':first-child': 0 },
    borderColor: themeColors['--color-popover-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule23: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: themeColors['--color-muted'],
    color: themeColors['--color-foreground-alt'],
  },
  rule24: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule25: {
    display: 'flex',
    minWidth: 0,
    flexWrap: 'wrap',
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 3)`,
    rowGap: themeLayout['--spacing'],
  },
  rule26: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule27: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule28: { color: themeColors['--color-green-500'] },
  rule29: { color: themeColors['--color-destructive'] },
  rule30: { color: themeColors['--color-foreground-alt'] },
  rule31: {
    width: 'fit-content',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand-highlight'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule32: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule33: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule34: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule35: { minWidth: 0, flex: 1 },
  rule36: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule37: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-green-500'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: { color: themeColors['--color-destructive'] },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// Auto-dismiss windows for the anchored feedback popover. Completion lingers
// slightly longer so the "added" confirmation reads before it fades; both are
// capped by the indicator itself unmounting once the manager clears its items.
const STARTED_DISMISS_MS = 3500
const COMPLETED_DISMISS_MS = 5000

// formatBytes formats a byte count into a human-readable string.
function formatBytes(bytes: number, decimals = 1): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}

function formatUploadError(error: string | undefined): string | undefined {
  if (!error || !isStorageQuotaError(error)) {
    return error
  }
  const available = error.match(/available (\d+) bytes/)
  const required = error.match(/write (\d+) bytes/)
  const headroom = available
    ? ` (${formatBytes(Number(available[1]))} available${
        required ? `; ${formatBytes(Number(required[1]))} needed` : ''
      })`
    : ''
  return `Browser storage quota was exceeded${headroom}. Free storage used by this site or device, then retry. Browser cleanup protection does not increase capacity.`
}

// UploadProgressOverlay renders the list of upload items with progress.
function UploadProgressOverlay({
  uploadManager,
  onOpenStorageHealth,
}: {
  uploadManager: UploadManager
  onOpenStorageHealth?: () => void
}) {
  const queuedCount = uploadManager.items.filter(
    (i) => i.status === 'queued',
  ).length
  const activeCount = uploadManager.items.filter(
    (i) => i.status === 'uploading',
  ).length
  const doneCount = uploadManager.items.filter(
    (i) => i.status === 'done',
  ).length
  const errorCount = uploadManager.items.filter(
    (i) => i.status === 'error',
  ).length
  const totalBytes = uploadManager.items.reduce(
    (sum, item) => sum + item.totalSize,
    0,
  )
  const writtenBytes = uploadManager.items.reduce(
    (sum, item) =>
      sum + (item.status === 'done' ? item.totalSize : item.bytesWritten),
    0,
  )
  const overallProgress =
    totalBytes > 0 ? Math.round((writtenBytes / totalBytes) * 100) : 0

  return (
    <div data-testid="upload-progress-overlay" sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <h2 sx={stylexStyles3.rule4}>Uploads</h2>
          <p sx={stylexStyles3.rule5}>
            Track file and folder uploads for this UnixFS view.
          </p>
        </div>
        <div sx={stylexStyles3.rule6}>
          {uploadManager.items.some((i) => i.status === 'done') && (
            <button
              type="button"
              sx={stylexStyles3.rule7}
              onClick={uploadManager.clearDone}
            >
              Clear done
            </button>
          )}
          {uploadManager.items.some(
            (i) => i.status === 'queued' || i.status === 'uploading',
          ) && (
            <button
              type="button"
              sx={stylexStyles3.rule8}
              onClick={uploadManager.cancelAll}
            >
              Cancel all
            </button>
          )}
        </div>
      </div>

      <div sx={stylexStyles3.rule9}>
        <SummaryCell label="Overall" value={`${overallProgress}%`} />
        <SummaryCell
          label="Active"
          value={
            activeCount > 0
              ? `${activeCount} uploading`
              : queuedCount > 0
                ? `${queuedCount} queued`
                : 'Idle'
          }
        />
        <SummaryCell label="Completed" value={`${doneCount}`} />
        <SummaryCell label="Failed" value={`${errorCount}`} />
      </div>

      <div sx={stylexStyles3.rule10}>
        <div sx={stylexStyles3.rule11}>
          <div
            sx={stylexStyles3.rule12}
            style={{ '--upload-progress-width': `${overallProgress}%` }}
          />
        </div>
        <div sx={stylexStyles3.rule13}>
          <span>
            {formatBytes(writtenBytes)} of {formatBytes(totalBytes)}
          </span>
          <span>{uploadManager.items.length} items</span>
        </div>
      </div>

      <div data-testid="upload-progress-list" sx={stylexStyles3.rule14}>
        {uploadManager.items.map((item) => (
          <UploadItemRow
            key={item.id}
            item={item}
            onCancel={uploadManager.cancelUpload}
            onOpenStorageHealth={onOpenStorageHealth}
          />
        ))}
      </div>
    </div>
  )
}

function SummaryCell({ label, value }: { label: string; value: string }) {
  return (
    <div sx={stylexStyles3.rule15}>
      <div sx={stylexStyles3.rule16}>{label}</div>
      <div sx={stylexStyles3.rule17}>{value}</div>
    </div>
  )
}

/** uploadStatusLabel returns the status text of an upload item. */
function uploadStatusLabel(item: UploadItem): string {
  switch (item.status) {
    case 'uploading':
      return 'Uploading'
    case 'queued':
      return 'Queued'
    case 'done':
      return 'Complete'
    default:
      return formatUploadError(item.error) ?? 'Failed'
  }
}

/** UploadItemAction shows the cancel, complete, or dismiss control of an upload item. */
function UploadItemAction({
  item,
  onCancel,
}: {
  item: UploadItem
  onCancel: (id: string) => void
}) {
  const handleCancel = useCallback(() => {
    onCancel(item.id)
  }, [item.id, onCancel])

  if (item.status === 'done') {
    return <LuCheck sx={stylexStyles.rule1} />
  }
  const dismissing = item.status === 'error'
  return (
    <button
      type="button"
      sx={stylexStyles3.rule18}
      onClick={handleCancel}
      title={dismissing ? 'Dismiss' : 'Cancel'}
    >
      <LuX sx={[stylexStyles.rule2, !!dismissing && stylexStyles.rule3]} />
    </button>
  )
}

/** UploadItemProgress renders the progress bar with the byte and percent counts. */
function UploadItemProgress({ item }: { item: UploadItem }) {
  const progress =
    item.totalSize > 0
      ? Math.round((item.bytesWritten / item.totalSize) * 100)
      : 0

  return (
    <div sx={stylexStyles3.rule19}>
      <div sx={stylexStyles3.rule20}>
        <div
          sx={stylexStyles3.rule12}
          style={{ '--upload-progress-width': `${progress}%` }}
        />
      </div>
      <span sx={stylexStyles3.rule21}>
        {formatBytes(
          item.status === 'done' ? item.totalSize : item.bytesWritten,
        )}
        {' / '}
        {formatBytes(item.totalSize)}
      </span>
      <span sx={stylexStyles3.rule21}>{progress}%</span>
    </div>
  )
}

// UploadItemRow renders a single upload item row.
function UploadItemRow({
  item,
  onCancel,
  onOpenStorageHealth,
}: {
  item: UploadItem
  onCancel: (id: string) => void
  onOpenStorageHealth?: () => void
}) {
  const quotaError = isStorageQuotaError(item.error)

  return (
    <div sx={stylexStyles3.rule22}>
      <div sx={stylexStyles3.rule23}>
        {item.kind === 'directory' ? (
          <LuFolder sx={stylexStyles.rule2} />
        ) : (
          <LuUpload sx={stylexStyles.rule2} />
        )}
      </div>
      <div sx={stylexStyles3.rule24}>
        <div sx={stylexStyles3.rule25}>
          <span sx={stylexStyles3.rule26}>{item.name}</span>
          <span sx={stylexStyles3.rule16}>{item.kind}</span>
          <span
            sx={[
              stylexStyles3.rule27,
              item.status === 'done' && stylexStyles3.rule28,
              item.status === 'error' && stylexStyles3.rule29,
              !!(item.status === 'queued' || item.status === 'uploading') &&
                stylexStyles3.rule30,
            ]}
          >
            {uploadStatusLabel(item)}
          </span>
        </div>
        {quotaError && onOpenStorageHealth && (
          <button
            type="button"
            sx={stylexStyles3.rule31}
            onClick={onOpenStorageHealth}
          >
            View storage health
          </button>
        )}
        <div sx={stylexStyles3.rule32}>{item.path}</div>
        <UploadItemProgress item={item} />
      </div>
      <div sx={stylexStyles3.rule33}>
        <UploadItemAction item={item} onCancel={onCancel} />
      </div>
    </div>
  )
}

type UploadFeedbackTone = 'started' | 'complete' | 'errors'

interface UploadFeedbackCopy {
  tone: UploadFeedbackTone
  title: string
  detail: string
}

/** uploadFeedbackCopy returns the tone and text for an upload lifecycle event. */
function uploadFeedbackCopy(feedback: UploadEvent): UploadFeedbackCopy {
  const files = feedback.fileCount === 1 ? 'file' : 'files'
  if (feedback.kind === 'started') {
    return {
      tone: 'started',
      title: 'Upload started',
      detail: `Uploading ${feedback.fileCount} ${files}. Track progress here.`,
    }
  }
  if (feedback.errorCount > 0) {
    return {
      tone: 'errors',
      title: 'Upload finished with errors',
      detail: `${feedback.fileCount} uploaded, ${feedback.errorCount} failed.`,
    }
  }
  return {
    tone: 'complete',
    title: 'Upload complete',
    detail: `${feedback.fileCount} ${files} added to this folder.`,
  }
}

const feedbackToneStyles: Record<UploadFeedbackTone, stylex.StyleXStyles> = {
  started: stylexStyles5.rule1,
  complete: stylexStyles5.rule2,
  errors: stylexStyles5.rule3,
}

const FeedbackToneIcon: Record<UploadFeedbackTone, IconType> = {
  started: LuUpload,
  complete: LuCheck,
  errors: LuCircleAlert,
}

// UploadFeedbackContent renders the anchored start/completion notification copy.
function UploadFeedbackContent({ feedback }: { feedback: UploadEvent }) {
  const { tone, title, detail } = uploadFeedbackCopy(feedback)
  const ToneIcon = FeedbackToneIcon[tone]

  return (
    <div sx={stylexStyles3.rule34} data-testid="upload-feedback">
      <div sx={[[stylexStyles6.rule1], [feedbackToneStyles[tone]]]}>
        <ToneIcon sx={stylexStyles2.rule1} />
      </div>
      <div sx={stylexStyles3.rule35}>
        <div sx={stylexStyles3.rule36}>{title}</div>
        <div sx={stylexStyles3.rule37}>{detail}</div>
      </div>
    </div>
  )
}

// UploadProgressBottomBar renders an upload progress indicator in the bottom bar.
export function UploadProgressBottomBar({
  uploadManager,
  onOpenStorageHealth,
}: {
  uploadManager: UploadManager
  onOpenStorageHealth?: () => void
}) {
  const totalCount = uploadManager.items.length
  const activeUploading = uploadManager.activeCount
  const doneCount = useMemo(
    () => uploadManager.items.filter((i) => i.status === 'done').length,
    [uploadManager.items],
  )

  // Surface each upload-lifecycle event as a temporary popover anchored to the
  // indicator, then auto-dismiss. Deduping on event id keeps a re-render from
  // reopening a dismissed notification.
  const [feedback, setFeedback] = useState<UploadEvent | null>(null)
  const seenEventIdRef = useRef(0)
  const lastEvent = uploadManager.lastEvent
  useEffect(() => {
    if (!lastEvent || lastEvent.id === seenEventIdRef.current) return
    seenEventIdRef.current = lastEvent.id
    setFeedback(lastEvent)
    const dismissMs =
      lastEvent.kind === 'started' ? STARTED_DISMISS_MS : COMPLETED_DISMISS_MS
    const timer = setTimeout(() => setFeedback(null), dismissMs)
    return () => clearTimeout(timer)
  }, [lastEvent])

  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <Popover open={feedback !== null}>
        <PopoverAnchor asChild>
          <button
            type="button"
            onClick={onClick}
            sx={[stylexStyles4.rule1, !!selected && stylexStyles4.rule2, sx]}
          >
            {activeUploading > 0 ? (
              <Spinner size="sm" />
            ) : (
              <LuUpload sx={stylexStyles.rule4} />
            )}
            {activeUploading > 0
              ? `Uploading ${activeUploading}/${totalCount}`
              : `${doneCount}/${totalCount} uploaded`}
          </button>
        </PopoverAnchor>
        {feedback !== null && (
          <PopoverContent
            side="top"
            align="end"
            sideOffset={6}
            onOpenAutoFocus={(e) => e.preventDefault()}
            onCloseAutoFocus={(e) => e.preventDefault()}
            data-testid="upload-feedback-popover"
            variant="compact"
          >
            <UploadFeedbackContent feedback={feedback} />
          </PopoverContent>
        )}
      </Popover>
    ),
    [activeUploading, totalCount, doneCount, feedback],
  )

  if (totalCount === 0) return null

  return (
    <BottomBarLevel
      id="upload-progress"
      position="right"
      button={buttonRender}
      overlay={
        <UploadProgressOverlay
          uploadManager={uploadManager}
          onOpenStorageHealth={onOpenStorageHealth}
        />
      }
    >
      {null}
    </BottomBarLevel>
  )
}
