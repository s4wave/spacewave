import { useState } from 'react'
import { LuPlus, LuShieldAlert } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

// WindowsBypassNotice renders a collapsible "How to run on Windows" block
// describing how to bypass SmartScreen for the interim unsigned build.
// Collapsed by default. Keyboard accessible as a <button> disclosure.
export function WindowsBypassNotice() {
  const [isOpen, setOpen] = useState(false)

  return (
    <div
      className={cn(
        'rounded-lg border backdrop-blur-sm transition-all',
        isOpen ?
          'border-foreground/12 bg-background-card/60'
        : 'border-foreground/6 bg-background-card/30 hover:border-foreground/12',
      )}
    >
      <button
        type="button"
        aria-expanded={isOpen}
        onClick={() => setOpen((v) => !v)}
        className="group flex w-full cursor-pointer items-start justify-between gap-4 p-5 text-left select-none"
      >
        <div className="flex items-start gap-3">
          <LuShieldAlert className="text-brand mt-0.5 h-5 w-5 shrink-0" />
          <h3 className="text-foreground group-hover:text-brand text-sm leading-relaxed font-semibold transition-colors @lg:text-base">
            How to run on Windows
          </h3>
        </div>
        <div
          className={cn(
            'mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md transition-all',
            isOpen ?
              'bg-brand/12 text-brand rotate-45'
            : 'bg-foreground/6 text-foreground-alt group-hover:bg-brand/8 group-hover:text-brand',
          )}
        >
          <LuPlus className="h-3 w-3" />
        </div>
      </button>
      <div
        className={cn(
          'grid transition-all duration-300 ease-in-out',
          isOpen ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
        )}
      >
        <div className="overflow-hidden">
          <div className="text-foreground-alt flex flex-col gap-3 px-5 pb-5 text-sm leading-relaxed">
            <ol className="ml-4 list-decimal space-y-2">
              <li>Click Download, save the zip, and extract it.</li>
              <li>
                Run <span className="font-mono text-xs">spacewave.exe</span>. If
                SmartScreen shows{' '}
                <strong className="text-foreground">
                  &quot;Windows protected your PC&quot;
                </strong>
                , click <strong className="text-foreground">More info</strong>,
                then <strong className="text-foreground">Run anyway</strong>.
              </li>
              <li>Only needed once per install.</li>
            </ol>
            <p className="text-xs">
              The Windows build is unsigned during Microsoft&apos;s code-signing
              identity validation. The signed MSIX build returns once validation
              completes; the SmartScreen prompt disappears at that point.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
