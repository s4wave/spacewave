import { LuCheck, LuShieldQuestion, LuX } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import type { TerminalFrame } from '@s4wave/sdk/terminal/terminal.pb.js'

export interface TerminalSshTrustPanelProps {
  challenge: TerminalFrame
  onRespond: (accepted: boolean) => void
}

export function TerminalSshTrustPanel({
  challenge,
  onRespond,
}: TerminalSshTrustPanelProps) {
  return (
    <div
      aria-label="SSH host key trust"
      className="border-warning/30 bg-background-card/80 text-foreground rounded-lg border px-4 py-3 text-sm backdrop-blur-sm"
      role="dialog"
    >
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0 space-y-2">
          <div className="text-warning flex items-center gap-1.5 text-xs font-semibold select-none">
            <LuShieldQuestion className="size-3.5 shrink-0" />
            Confirm SSH host key
          </div>
          <div className="grid gap-1 text-xs md:grid-cols-[8rem_minmax(0,1fr)]">
            <span className="text-foreground-alt/60 select-none">Host</span>
            <span className="text-foreground font-mono break-all">
              {challenge.sshTrustHost || 'unknown'}
            </span>
            <span className="text-foreground-alt/60 select-none">
              Algorithm
            </span>
            <span className="text-foreground font-mono break-all">
              {challenge.sshTrustAlgorithm || 'unknown'}
            </span>
            <span className="text-foreground-alt/60 select-none">SHA256</span>
            <span className="text-foreground font-mono break-all">
              {challenge.sshTrustSha256Fingerprint || 'unknown'}
            </span>
            <span className="text-foreground-alt/60 select-none">
              Public key
            </span>
            <span className="text-foreground font-mono break-all">
              {challenge.sshTrustPublicKey || 'unknown'}
            </span>
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            className={cn(
              'border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground',
              'h-7 gap-1.5 px-3 text-xs font-medium transition-all duration-150 select-none',
            )}
            onClick={() => onRespond(true)}
            size="sm"
            type="button"
            variant="outline"
          >
            <LuCheck className="size-3.5" />
            Trust
          </Button>
          <DashboardButton
            icon={<LuX className="size-3.5" />}
            onClick={() => onRespond(false)}
          >
            Reject
          </DashboardButton>
        </div>
      </div>
    </div>
  )
}
