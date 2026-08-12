import { LuDownload } from 'react-icons/lu'

import { ExternalLink } from '@s4wave/app/landing/ExternalLink.js'
import { cn } from '@s4wave/web/style/utils.js'

import type { DownloadEntry } from './manifest.js'

interface PrimaryDownloadButtonProps {
  entry: DownloadEntry | null
  releaseLabel?: string
}

// PrimaryDownloadButton renders the hero download CTA for the detected
// platform. Falls back to a "Pick a build below." headline when detection
// missed or the detected tuple is absent from the manifest.
export function PrimaryDownloadButton({
  entry,
  releaseLabel,
}: PrimaryDownloadButtonProps) {
  if (!entry) {
    return (
      <p className="text-foreground-alt text-center text-base select-none @lg:text-lg">
        Pick a build below.
      </p>
    )
  }

  return (
    <ExternalLink
      href={entry.url}
      download
      className={cn(
        'border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15',
        'inline-flex cursor-pointer items-center gap-3 rounded-lg border px-6 py-3 text-base font-semibold select-none',
        'transition-all duration-300 hover:-translate-y-0.5 @lg:text-lg',
      )}
    >
      <LuDownload className="size-5" />
      <span>
        Download{releaseLabel ? ` ${releaseLabel}` : ''} for {entry.osLabel} (
        {entry.archLabel}) · {entry.ext}
      </span>
    </ExternalLink>
  )
}
