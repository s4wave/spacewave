import * as ownerStylex from '@stylexjs/stylex'
import { cn as ownerClasses } from '@s4wave/web/style/utils.js'
import { groupMarker as ownergroupMarker } from '@s4wave/web/style/relations.stylex.js'
import { LuDownload } from '@s4wave/web/images/icons.js'

import { ExternalLink } from '@s4wave/app/landing/ExternalLink.js'
import { cn } from '@s4wave/web/style/utils.js'

import type { DownloadEntry } from './manifest.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

interface ArchTileProps {
  entry: DownloadEntry
}

// ArchTile renders one per-architecture download option with arch label,
// filename, and a download link.
export function ArchTile({ entry }: ArchTileProps) {
  return (
    <ExternalLink
      href={entry.url}
      download
      className={cn(
        'border-foreground/6 bg-background-card/30 hover:border-foreground/12',
        ownerClasses(
          'group flex cursor-pointer flex-col gap-2 rounded-lg border p-6',
          ownerStylex.props(ownergroupMarker).className,
        ),
        'transition-all duration-300 hover:-translate-y-0.5',
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-foreground group-hover:text-brand text-base font-semibold transition-colors select-none">
          {entry.archLabel}
        </span>
        <LuDownload sx={stylexStyles.rule1} />
      </div>
      <span className="text-foreground-alt font-mono text-xs break-all select-none">
        {entry.filename}
      </span>
    </ExternalLink>
  )
}
