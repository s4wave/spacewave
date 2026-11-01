import { LuTriangleAlert } from 'react-icons/lu'

// VmErrorBannerProps configures an error notice.
export interface VmErrorBannerProps {
  title: string
  message: string
}

// VmErrorBanner states a VM or request failure in a red notice.
export function VmErrorBanner({ title, message }: VmErrorBannerProps) {
  return (
    <div
      role="alert"
      className="border-destructive/20 bg-destructive/5 text-destructive flex items-start gap-2 rounded-md border px-3 py-2 text-xs"
    >
      <LuTriangleAlert className="mt-px size-3.5 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="font-medium">{title}</div>
        <div className="text-foreground-alt break-words">{message}</div>
      </div>
    </div>
  )
}
