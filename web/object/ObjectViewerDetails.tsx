import { useCallback, useState } from 'react'
import { LuX, LuEye, LuInfo, LuDownload, LuTrash2 } from 'react-icons/lu'
import { RxCube } from 'react-icons/rx'

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
    <div className="bg-background-primary flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <div className="text-foreground flex min-w-0 items-center gap-2 text-sm font-semibold select-none">
          <RxCube className="size-4 shrink-0" />
          <span className="truncate tracking-tight">{objectKey}</span>
          <span className="text-foreground-alt shrink-0">· Object</span>
        </div>
        {onCloseClick && (
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuX className="size-4" />}
                onClick={onCloseClick}
              />
            </TooltipTrigger>
            <TooltipContent side="bottom">Close</TooltipContent>
          </Tooltip>
        )}
      </div>

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto w-full max-w-5xl">
          <div className="grid gap-4 lg:grid-cols-2">
            <div className="min-w-0 space-y-3">
              <section>
                <div className="mb-2 flex items-center justify-between">
                  <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
                    <LuInfo className="size-3.5" />
                    About this object
                  </h2>
                </div>

                <InfoCard>
                  <div className="space-y-3">
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
                icon={<LuInfo className="size-3.5" />}
                open={technicalDetailsOpen}
                onOpenChange={setTechnicalDetailsOpen}
              >
                <div className="space-y-2">
                  <CopyableField label="Type ID" value={typeID} />
                  {rootRef && (
                    <CopyableField label="Root Ref" value={rootRef} />
                  )}
                </div>
              </CollapsibleSection>
            </div>

            <div className="min-w-0 space-y-3">
              {exportUrl && <ExportDataSection exportUrl={exportUrl} />}

              {availableComponents.length > 0 && (
                <section>
                  <div className="mb-2 flex items-center justify-between">
                    <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
                      <LuEye className="size-3.5" />
                      Viewer
                    </h2>
                  </div>

                  <InfoCard>
                    <div className="space-y-1.5">
                      {availableComponents.map((component) => {
                        const isSelected =
                          displayedSelectedComponentID === component.componentID
                        return (
                          <button
                            key={component.componentID}
                            type="button"
                            aria-pressed={isSelected}
                            onClick={() => handleComponentSelect(component)}
                            className={cn(
                              'border-foreground/10 hover:border-foreground/20 hover:bg-foreground/5 flex w-full cursor-pointer items-start justify-between gap-3 rounded-lg border p-2.5 text-left transition-colors',
                              isSelected && 'border-brand/30 bg-brand/10',
                            )}
                          >
                            <div className="min-w-0 flex-1">
                              <div className="flex min-w-0 items-center gap-2">
                                <p className="text-foreground text-sm font-medium select-none">
                                  {component.name}
                                </p>
                                {isSelected && (
                                  <span className="border-brand/30 bg-brand/10 text-brand shrink-0 rounded border px-1.5 py-0.5 text-[0.65rem] font-medium select-none">
                                    Active
                                  </span>
                                )}
                              </div>
                              <p className="text-foreground-alt/50 font-mono text-[0.6rem] break-all select-none">
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
                        className="border-brand/30 bg-brand/10 text-brand hover:bg-brand/15 mt-3 flex h-7 w-full cursor-pointer items-center justify-center gap-1.5 rounded-md border text-xs font-medium transition-colors"
                      >
                        <LuEye className="size-3.5" />
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
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuDownload className="size-3.5" />
          Data
        </h2>
      </div>

      <div className="space-y-2">
        <button
          type="button"
          onClick={() => void handleExport()}
          disabled={status === 'busy' || status === 'success'}
          aria-busy={status === 'busy'}
          className="border-foreground/10 hover:border-foreground/20 hover:bg-foreground/5 flex h-14 w-full cursor-pointer items-center gap-3 rounded-lg border p-2.5 text-left transition-colors disabled:cursor-default disabled:opacity-100"
        >
          <div className="bg-foreground/5 flex size-8 shrink-0 items-center justify-center rounded-md">
            <LuDownload className="text-foreground size-3.5" />
          </div>
          <div className="flex min-w-0 flex-1 flex-col">
            <h4 className="text-foreground text-sm font-medium select-none">
              {status === 'busy'
                ? 'Preparing export…'
                : status === 'success'
                  ? 'Export ready'
                  : status === 'failure'
                    ? 'Export could not be prepared'
                    : 'Export Data'}
            </h4>
            <p className="text-foreground-alt text-xs leading-relaxed select-none">
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
          <div className="border-destructive/15 bg-destructive/5 flex items-center justify-between gap-3 rounded-lg border p-2.5 text-xs">
            <p className="text-destructive leading-relaxed">
              Export could not be prepared.
            </p>
            <button
              type="button"
              onClick={() => void handleExport()}
              className="text-destructive hover:bg-destructive/10 shrink-0 rounded-md px-2 py-1 font-medium transition-colors"
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
          className="border-destructive/30 bg-destructive/5 hover:border-destructive hover:bg-destructive/10 group flex w-full cursor-pointer items-center gap-3 rounded-lg border p-2.5 text-left transition-colors"
        >
          <div className="bg-destructive/20 group-hover:bg-destructive/30 flex size-8 shrink-0 items-center justify-center rounded-md transition-colors">
            <LuTrash2 className="text-destructive size-3.5" />
          </div>
          <div className="flex min-w-0 flex-1 flex-col">
            <h4 className="text-destructive text-xs font-medium select-none">
              Delete Object
            </h4>
            <p className="text-destructive/80 text-xs select-none">
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

          {error && <p className="text-destructive text-xs">{error}</p>}

          <DialogFooter>
            <button
              type="button"
              onClick={() => handleDialogOpenChange(false)}
              disabled={submitting}
              className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void handleDelete()}
              disabled={submitting}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-all',
                'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              {submitting ? 'Deleting…' : 'Delete Object'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
