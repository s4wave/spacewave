import { LuCheck, LuX } from 'react-icons/lu'

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
      className="border-amber-300/40 bg-amber-50 px-4 py-3 text-sm text-amber-950 shadow-sm dark:bg-amber-950 dark:text-amber-50"
      role="dialog"
    >
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0 space-y-2">
          <div className="font-semibold">Confirm SSH host key</div>
          <div className="grid gap-1 text-xs md:grid-cols-[8rem_minmax(0,1fr)]">
            <span className="text-amber-800 dark:text-amber-200">Host</span>
            <span className="font-mono break-all">
              {challenge.sshTrustHost || 'unknown'}
            </span>
            <span className="text-amber-800 dark:text-amber-200">
              Algorithm
            </span>
            <span className="font-mono break-all">
              {challenge.sshTrustAlgorithm || 'unknown'}
            </span>
            <span className="text-amber-800 dark:text-amber-200">SHA256</span>
            <span className="font-mono break-all">
              {challenge.sshTrustSha256Fingerprint || 'unknown'}
            </span>
            <span className="text-amber-800 dark:text-amber-200">
              Public key
            </span>
            <span className="font-mono break-all">
              {challenge.sshTrustPublicKey || 'unknown'}
            </span>
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          <button
            className="inline-flex h-8 items-center gap-1.5 rounded-md bg-emerald-600 px-3 text-xs font-semibold text-white hover:bg-emerald-700"
            onClick={() => onRespond(true)}
            type="button"
          >
            <LuCheck className="size-3.5" />
            Trust
          </button>
          <button
            className="inline-flex h-8 items-center gap-1.5 rounded-md border border-amber-300/70 px-3 text-xs font-semibold text-amber-950 hover:bg-amber-100 dark:text-amber-50 dark:hover:bg-amber-900"
            onClick={() => onRespond(false)}
            type="button"
          >
            <LuX className="size-3.5" />
            Reject
          </button>
        </div>
      </div>
    </div>
  )
}
