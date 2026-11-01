import { useCallback, useState } from 'react'
import {
  LuX,
  LuEye,
  LuInfo,
  LuDownload,
  LuTrash2,
} from '@s4wave/web/images/icons.js'
import { RxCube } from '@s4wave/web/images/icons.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { downloadURL } from '@s4wave/web/download.js'
import { cn } from '@s4wave/web/style/utils.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import type { ObjectViewerComponent } from './object.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
  rule5: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule6: { flexShrink: 0, color: themeColors['--color-foreground-alt'] },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule9: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
  },
  rule10: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 64rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule11: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule12: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule13: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule14: { minWidth: 0, flex: 1 },
  rule15: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule16: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule17: {
    flexShrink: 0,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.65rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule18: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.6rem',
    lineHeight: 1.5,
    wordBreak: 'break-all',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule19: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule20: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 14)`,
    width: '100%',
    cursor: { default: 'pointer', ':disabled': 'default' },
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    opacity: { default: null, ':disabled': 1 },
  },
  rule21: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule22: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground'],
  },
  rule23: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule24: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule25: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule26: {
    '--tw-leading': 'var(--leading-relaxed)',
    lineHeight: 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
  },
  rule27: {
    flexShrink: 0,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  rule28: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-destructive'],
  },
  rule29: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule30: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule31: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule32: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
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
  rule33: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

export interface ObjectViewerDetailsProps {
  objectKey: string
  typeID: string
  rootRef?: string
  exportUrl?: string
  availableComponents: ObjectViewerComponent[]
  selectedComponent?: ObjectViewerComponent
  missingComponentID?: string
  onComponentSelect: (component: ObjectViewerComponent) => void
  onCloseClick?: () => void
  onDeleteConfirm?: () => Promise<void>
}

// ObjectViewerDetails displays metadata and viewer selection for an object.
export function ObjectViewerDetails({
  objectKey,
  typeID,
  rootRef,
  exportUrl,
  availableComponents,
  selectedComponent,
  missingComponentID,
  onComponentSelect,
  onCloseClick,
  onDeleteConfirm,
}: ObjectViewerDetailsProps) {
  const [technicalDetailsOpen, setTechnicalDetailsOpen] = useState(false)
  const [selectedComponentOverride, setSelectedComponentOverride] = useState<
    string | undefined
  >()

  const displayedSelectedComponentID =
    selectedComponentOverride ?? selectedComponent?.componentID
  const displayedSelectedComponent =
    availableComponents.find(
      (component) => component.componentID === displayedSelectedComponentID,
    ) ?? selectedComponent

  const handleComponentSelect = useCallback(
    (component: ObjectViewerComponent) => {
      setSelectedComponentOverride(component.componentID)
      onComponentSelect(component)
    },
    [onComponentSelect],
  )

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <RxCube {...stylex.props(stylexStyles.rule4)} />
          <span sx={stylexStyles.rule5}>{objectKey}</span>
          <span sx={stylexStyles.rule6}>· Object</span>
        </div>
        {onCloseClick && (
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuX {...stylex.props(stylexStyles.rule7)} />}
                onClick={onCloseClick}
              />
            </TooltipTrigger>
            <TooltipContent side="bottom">Close</TooltipContent>
          </Tooltip>
        )}
      </div>

      <div sx={stylexStyles.rule8}>
        <div sx={stylexStyles.rule9}>
          <div sx={stylexStyles.rule10}>
            <div
              className={
                /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'min-w-0 space-y-3'
              }
            >
              <section>
                <div sx={stylexStyles.rule11}>
                  <h2 sx={stylexStyles.rule12}>
                    <LuInfo {...stylex.props(stylexStyles.rule13)} />
                    About this object
                  </h2>
                </div>

                <InfoCard>
                  <div
                    className={
                      /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
                    }
                  >
                    <CopyableField label="Object key" value={objectKey} />
                    {missingComponentID && (
                      <CopyableField
                        label="Missing Component ID"
                        value={missingComponentID}
                      />
                    )}
                  </div>
                </InfoCard>
              </section>

              <CollapsibleSection
                title="Technical details"
                icon={<LuInfo {...stylex.props(stylexStyles.rule13)} />}
                open={technicalDetailsOpen}
                onOpenChange={setTechnicalDetailsOpen}
              >
                <div
                  className={
                    /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
                  }
                >
                  <CopyableField label="Type ID" value={typeID} />
                  {rootRef && (
                    <CopyableField label="Root Ref" value={rootRef} />
                  )}
                </div>
              </CollapsibleSection>
            </div>

            <div
              className={
                /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'min-w-0 space-y-3'
              }
            >
              {exportUrl && <ExportDataSection exportUrl={exportUrl} />}

              {availableComponents.length > 0 && (
                <section>
                  <div sx={stylexStyles.rule11}>
                    <h2 sx={stylexStyles.rule12}>
                      <LuEye {...stylex.props(stylexStyles.rule13)} />
                      Viewer
                    </h2>
                  </div>

                  <InfoCard>
                    <div
                      className={
                        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-1.5 */ 'space-y-1.5'
                      }
                    >
                      {availableComponents.map((component) => {
                        const isSelected =
                          displayedSelectedComponentID === component.componentID
                        return (
                          <button
                            key={component.componentID}
                            type="button"
                            aria-pressed={isSelected}
                            onClick={() => handleComponentSelect(component)}
                            className={
                              /* TODO(stylex): conditional property composition: borderColor */ cn(
                                'border-foreground/10 hover:border-foreground/20 hover:bg-foreground/5 flex w-full cursor-pointer items-start justify-between gap-3 rounded-lg border p-2.5 text-left transition-colors',
                                isSelected && 'border-brand/30 bg-brand/10',
                              )
                            }
                          >
                            <div sx={stylexStyles.rule14}>
                              <div sx={stylexStyles.rule15}>
                                <p sx={stylexStyles.rule16}>{component.name}</p>
                                {isSelected && (
                                  <span sx={stylexStyles.rule17}>Active</span>
                                )}
                              </div>
                              <p sx={stylexStyles.rule18}>
                                ID: {component.componentID}
                              </p>
                            </div>
                          </button>
                        )
                      })}
                    </div>

                    {displayedSelectedComponent && onCloseClick && (
                      <button
                        type="button"
                        onClick={onCloseClick}
                        sx={stylexStyles.rule19}
                      >
                        <LuEye {...stylex.props(stylexStyles.rule13)} />
                        Open viewer
                      </button>
                    )}
                  </InfoCard>
                </section>
              )}

              {onDeleteConfirm && (
                <DangerZoneSection
                  objectKey={objectKey}
                  onDeleteConfirm={onDeleteConfirm}
                />
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ExportDataSection renders a download button for exporting object data.
function ExportDataSection({ exportUrl }: { exportUrl: string }) {
  const [status, setStatus] = useState<'idle' | 'busy' | 'success' | 'failure'>(
    'idle',
  )

  const handleExport = useCallback(async () => {
    if (status === 'busy') return

    setStatus('busy')
    try {
      await downloadURL(exportUrl)
      setStatus('success')
    } catch (err: unknown) {
      console.error('failed to export object data', err)
      setStatus('failure')
      toast.error('Export could not be prepared')
    }
  }, [exportUrl, status])

  return (
    <section>
      <div sx={stylexStyles.rule11}>
        <h2 sx={stylexStyles.rule12}>
          <LuDownload {...stylex.props(stylexStyles.rule13)} />
          Data
        </h2>
      </div>

      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
        }
      >
        <button
          type="button"
          onClick={() => void handleExport()}
          disabled={status === 'busy' || status === 'success'}
          aria-busy={status === 'busy'}
          sx={stylexStyles.rule20}
        >
          <div sx={stylexStyles.rule21}>
            <LuDownload {...stylex.props(stylexStyles.rule22)} />
          </div>
          <div sx={stylexStyles.rule23}>
            <h4 sx={stylexStyles.rule16}>
              {status === 'busy'
                ? 'Preparing export…'
                : status === 'success'
                  ? 'Export ready'
                  : status === 'failure'
                    ? 'Export could not be prepared'
                    : 'Export Data'}
            </h4>
            <p sx={stylexStyles.rule24}>
              {status === 'busy'
                ? 'Preparing a downloadable archive…'
                : status === 'success'
                  ? 'The object contents are ready to download.'
                  : status === 'failure'
                    ? 'Try again to prepare the export.'
                    : 'Download object contents as zip'}
            </p>
          </div>
        </button>

        {status === 'failure' && (
          <div sx={stylexStyles.rule25}>
            <p sx={stylexStyles.rule26}>Export could not be prepared.</p>
            <button
              type="button"
              onClick={() => void handleExport()}
              sx={stylexStyles.rule27}
            >
              Try again
            </button>
          </div>
        )}
      </div>
    </section>
  )
}

interface DangerZoneSectionProps {
  objectKey: string
  onDeleteConfirm: () => Promise<void>
}

function DangerZoneSection({
  objectKey,
  onDeleteConfirm,
}: DangerZoneSectionProps) {
  const [open, setOpen] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()

  const handleDialogOpenChange = useCallback((next: boolean) => {
    if (!next) {
      setError(undefined)
      setSubmitting(false)
    }
    setDialogOpen(next)
  }, [])

  const handleDelete = useCallback(async () => {
    setSubmitting(true)
    setError(undefined)
    try {
      await onDeleteConfirm()
      handleDialogOpenChange(false)
    } catch (err) {
      console.error('failed to delete object', err)
      setError('Delete could not be completed. Try again.')
      setSubmitting(false)
    }
  }, [handleDialogOpenChange, onDeleteConfirm])

  return (
    <>
      <CollapsibleSection
        title="Danger Zone"
        open={open}
        onOpenChange={setOpen}
      >
        <button
          type="button"
          onClick={() => setDialogOpen(true)}
          className={
            /* TODO(stylex): global or unknown class: group */ 'border-destructive/30 bg-destructive/5 hover:border-destructive hover:bg-destructive/10 group flex w-full cursor-pointer items-center gap-3 rounded-lg border p-2.5 text-left transition-colors'
          }
        >
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: group-hover:bg-destructive/30 */ 'bg-destructive/20 group-hover:bg-destructive/30 flex size-8 shrink-0 items-center justify-center rounded-md transition-colors'
            }
          >
            <LuTrash2 {...stylex.props(stylexStyles.rule28)} />
          </div>
          <div sx={stylexStyles.rule23}>
            <h4 sx={stylexStyles.rule29}>Delete Object</h4>
            <p sx={stylexStyles.rule30}>
              Permanently remove this object and all its data
            </p>
          </div>
        </button>
      </CollapsibleSection>

      <Dialog open={dialogOpen} onOpenChange={handleDialogOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Object</DialogTitle>
            <DialogDescription>
              This will permanently delete &ldquo;{objectKey}&rdquo; and all its
              data.
            </DialogDescription>
          </DialogHeader>

          {error && <p sx={stylexStyles.rule31}>{error}</p>}

          <DialogFooter>
            <button
              type="button"
              onClick={() => handleDialogOpenChange(false)}
              disabled={submitting}
              sx={stylexStyles.rule32}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void handleDelete()}
              disabled={submitting}
              sx={[stylexStyles.rule33]}
            >
              {submitting ? 'Deleting…' : 'Delete Object'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
