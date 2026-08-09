import type { ReactNode, Ref } from 'react'

export function GitViewerCenteredState({
  containerRef,
  title,
  subtitle,
  detail,
  action,
}: {
  containerRef?: Ref<HTMLDivElement>
  title: ReactNode
  subtitle?: ReactNode
  detail?: ReactNode
  action?: ReactNode
}) {
  return (
    <div
      ref={containerRef}
      className="flex h-full w-full flex-col items-center justify-center overflow-hidden"
    >
      <div className="text-foreground text-sm font-semibold">{title}</div>
      {subtitle && (
        <div className="text-foreground-alt mt-1 text-xs font-medium">
          {subtitle}
        </div>
      )}
      {detail && (
        <div className="text-foreground-alt/70 mt-2 text-xs">{detail}</div>
      )}
      {action}
    </div>
  )
}
