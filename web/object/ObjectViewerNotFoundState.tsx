import { LuCircleAlert } from 'react-icons/lu'

export interface ObjectViewerNotFoundStateProps {
  objectKey?: string
}

// ObjectViewerNotFoundState renders the shared missing-object viewer surface.
export function ObjectViewerNotFoundState({
  objectKey,
}: ObjectViewerNotFoundStateProps) {
  return (
    <div className="bg-background-primary flex h-full w-full flex-1 items-center justify-center p-4">
      <div className="border-foreground/6 bg-background-card/30 flex max-w-sm items-start gap-3 rounded-lg border p-3.5 backdrop-blur-sm">
        <div className="bg-foreground/5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md">
          <LuCircleAlert className="text-foreground-alt/50 h-4 w-4" />
        </div>
        <div className="min-w-0">
          <p className="text-foreground text-sm font-semibold tracking-tight select-none">
            Object not found
          </p>
          <p className="text-foreground-alt/60 mt-1 text-xs leading-relaxed break-words">
            {objectKey || 'The selected object'} is no longer available.
          </p>
        </div>
      </div>
    </div>
  )
}
