import type { ReactNode } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// PageWrapper renders the shared outer layout for plan pages.
export function PageWrapper({
  backButton,
  children,
}: {
  backButton?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center overflow-y-auto p-6 outline-none md:p-10">
      {backButton && (
        <div className="relative z-10 w-full max-w-2xl">{backButton}</div>
      )}
      <div
        className={cn(
          'relative z-10 my-auto flex w-full max-w-2xl flex-col gap-6',
          backButton && 'pt-6',
        )}
      >
        {children}
      </div>
    </div>
  )
}
