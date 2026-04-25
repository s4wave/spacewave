import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { DISCORD_INVITE_URL, GITHUB_REPO_URL } from '@s4wave/app/github.js'
import { useAppBuildInfo } from '@s4wave/app/build-info.js'
import { AppLogo } from '@s4wave/web/images/AppLogo.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

interface AboutDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

// AboutDialog renders a Photoshop-style about modal with app branding.
export function AboutDialog({ open, onOpenChange }: AboutDialogProps) {
  const buildInfo = useAppBuildInfo()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="relative gap-0 overflow-hidden p-0 sm:max-w-sm"
      >
        <DialogTitle className="sr-only">About Spacewave</DialogTitle>
        <DialogDescription className="sr-only">
          Free, open-source, local-first platform for file sync, communication,
          device management, and community plugins.
        </DialogDescription>
        <p className="text-foreground-alt/60 absolute top-5 right-6 text-right text-xs">
          {buildInfo.version}
        </p>

        {/* Header with logo */}
        <div className="flex flex-col items-center px-6 pt-8 pb-2">
          <AppLogo
            className="mb-4 size-20"
            style={{ padding: 0 }}
            alt="Spacewave"
          />
          <h2 className="text-xl font-semibold tracking-tight">Spacewave</h2>
          {buildInfo.runtimeLabel && (
            <p className="text-foreground-alt/40 mt-1 text-[10px]">
              {buildInfo.runtimeLabel}
            </p>
          )}
        </div>

        {/* Description */}
        <div className="px-6 pb-4 text-center">
          <p className="text-foreground-alt py-2 text-sm leading-relaxed">
            Free, open-source, local-first platform for file sync,
            communication, device management, and more with community plugins.
            End-to-end encrypted. Runs directly in the web browser.
          </p>
        </div>

        {/* Links */}
        <div className="border-border flex items-center justify-center gap-4 border-t px-6 py-3">
          <a
            href={GITHUB_REPO_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="text-foreground-alt hover:text-foreground text-xs transition-colors"
          >
            GitHub
          </a>
          <span className="text-foreground-alt/30 text-xs">|</span>
          <a
            href={DISCORD_INVITE_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="text-foreground-alt hover:text-foreground text-xs transition-colors"
          >
            Discord
          </a>
          <span className="text-foreground-alt/30 text-xs">|</span>
          <a
            href="#/docs"
            className="text-foreground-alt hover:text-foreground text-xs transition-colors"
          >
            Documentation
          </a>
        </div>

        {/* Footer */}
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              onClick={() => window.open('https://cjs.zip', '_blank')}
              className="bg-muted/30 border-border block cursor-pointer border-t px-6 py-3 text-center"
            >
              <p className="text-foreground-alt/50 text-[10px]">
                Created by Christian Stewart
              </p>
            </button>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            <div className="space-y-1 text-xs">
              <p>Dedicated to my father, Jim, and my cat, May.</p>
              <p>May they rest in peace ❤ ~ CJS, 2024</p>
            </div>
          </TooltipContent>
        </Tooltip>
      </DialogContent>
    </Dialog>
  )
}
