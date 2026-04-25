import { LuDownload } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import type { DownloadEntry } from './manifest.js'

interface ArchTileProps {
  entry: DownloadEntry
}

// ArchTile renders one per-architecture download option with arch label,
// filename, and a download link.
export function ArchTile({ entry }: ArchTileProps) {
  return (
    <a
      href={entry.url}
      download
      rel="noopener noreferrer"
      className={cn(
        'border-foreground/6 bg-background-card/30 hover:border-foreground/12',
        'group flex cursor-pointer flex-col gap-2 rounded-lg border p-6 backdrop-blur-sm',
        'transition-all duration-300 hover:-translate-y-0.5',
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-foreground group-hover:text-brand text-base font-semibold transition-colors select-none">
          {entry.archLabel}
        </span>
        <LuDownload className="text-foreground-alt group-hover:text-brand h-5 w-5 transition-colors" />
      </div>
      <span className="text-foreground-alt font-mono text-xs break-all select-none">
        {entry.filename}
      </span>
    </a>
  )
}
