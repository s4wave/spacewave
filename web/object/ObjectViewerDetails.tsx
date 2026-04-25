import { useCallback } from 'react'
import { LuX, LuEye, LuInfo, LuDownload } from 'react-icons/lu'
import { RxCube } from 'react-icons/rx'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { downloadURL } from '@s4wave/web/download.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import type { ObjectViewerComponent } from './object.js'

export interface ObjectViewerDetailsProps {
  objectKey: string
  typeID: string
  rootRef?: string
  exportUrl?: string
  availableComponents: ObjectViewerComponent[]
  selectedComponent?: ObjectViewerComponent
  onComponentSelect: (component: ObjectViewerComponent) => void
  onCloseClick?: () => void
}

// ObjectViewerDetails displays metadata and viewer selection for an object.
export function ObjectViewerDetails({
  objectKey,
  typeID,
  rootRef,
  exportUrl,
  availableComponents,
  selectedComponent,
  onComponentSelect,
  onCloseClick,
}: ObjectViewerDetailsProps) {
  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
          <RxCube className="h-4 w-4" />
          <span className="tracking-tight">{objectKey}</span>
          <span className="text-foreground-alt">· Object</span>
        </div>
        {onCloseClick && (
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuX className="h-4 w-4" />}
                onClick={onCloseClick}
              />
            </TooltipTrigger>
            <TooltipContent side="bottom">Close</TooltipContent>
          </Tooltip>
        )}
      </div>

      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="space-y-3">
          <section>
            <div className="mb-2 flex items-center justify-between">
              <h2 className="text-foreground flex items-center gap-1.5 text-xs select-none">
                <LuInfo className="h-3.5 w-3.5" />
                Details
              </h2>
            </div>

            <InfoCard>
              <div className="space-y-2">
                <CopyableField label="Object Key" value={objectKey} />
                <CopyableField label="Type ID" value={typeID} />
                {rootRef && <CopyableField label="Root Ref" value={rootRef} />}
              </div>
            </InfoCard>
          </section>

          {exportUrl && <ExportDataSection exportUrl={exportUrl} />}

          {availableComponents.length > 0 && (
            <section>
              <div className="mb-2 flex items-center justify-between">
                <h2 className="text-foreground flex items-center gap-1.5 text-xs select-none">
                  <LuEye className="h-3.5 w-3.5" />
                  Viewer Components
                </h2>
              </div>

              <InfoCard>
                <div className="space-y-1.5">
                  {availableComponents.map((component, idx) => {
                    const isSelected =
                      selectedComponent?.name === component.name
                    return (
                      <button
                        key={idx}
                        onClick={() => onComponentSelect(component)}
                        className={cn(
                          'border-foreground/8 hover:border-foreground/12 hover:bg-foreground/5 flex w-full cursor-pointer items-center justify-between rounded-lg border p-2.5 text-left transition-colors',
                          isSelected && 'bg-foreground/5 border-foreground/12',
                        )}
                      >
                        <div className="min-w-0 flex-1">
                          <p className="text-foreground text-xs select-none">
                            {component.name}
                          </p>
                          <p className="text-foreground-alt text-xs select-none">
                            Type: {component.typeID}
                          </p>
                        </div>
                        {isSelected && (
                          <span className="text-primary text-xs select-none">
                            Active
                          </span>
                        )}
                      </button>
                    )
                  })}
                </div>
              </InfoCard>
            </section>
          )}
        </div>
      </div>
    </div>
  )
}

// ExportDataSection renders a download button for exporting object data.
function ExportDataSection({ exportUrl }: { exportUrl: string }) {
  const handleExport = useCallback(() => {
    downloadURL(exportUrl)
  }, [exportUrl])

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground flex items-center gap-1.5 text-xs select-none">
          <LuDownload className="h-3.5 w-3.5" />
          Data
        </h2>
      </div>

      <button
        onClick={handleExport}
        className="border-foreground/8 hover:border-foreground/12 hover:bg-foreground/5 flex w-full cursor-pointer items-center gap-3 rounded-lg border p-2.5 text-left transition-colors"
      >
        <div className="bg-foreground/5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md">
          <LuDownload className="text-foreground h-3.5 w-3.5" />
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <h4 className="text-foreground text-xs select-none">Export Data</h4>
          <p className="text-foreground-alt text-xs select-none">
            Download object contents as zip
          </p>
        </div>
      </button>
    </section>
  )
}
