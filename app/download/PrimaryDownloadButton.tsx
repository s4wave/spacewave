import { LuDownload } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import type { DownloadEntry } from './manifest.js'

interface PrimaryDownloadButtonProps {
  entry: DownloadEntry | null
}

// PrimaryDownloadButton renders the hero download CTA for the detected
// platform. Falls back to a "Pick a build below." headline when detection
// missed or the detected tuple is absent from the manifest.
export function PrimaryDownloadButton({ entry }: PrimaryDownloadButtonProps) {
  if (!entry) {
    return (
      <p className="text-foreground-alt text-center text-base select-none @lg:text-lg">
        Pick a build below.
      </p>
    )
  }

  return (
    <a
      href={entry.url}
      download
      rel="noopener noreferrer"
      className={cn(
        'border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15',
        'inline-flex cursor-pointer items-center gap-3 rounded-lg border px-6 py-3 text-base font-semibold select-none',
        'transition-all duration-300 hover:-translate-y-0.5 @lg:text-lg',
      )}
    >
      <LuDownload className="h-5 w-5" />
      <span>
        Download for {entry.osLabel} ({entry.archLabel})
      </span>
    </a>
  )
}
