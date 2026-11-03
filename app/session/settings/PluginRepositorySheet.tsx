import { JsModuleKind } from '@go/github.com/s4wave/spacewave/bldr/plugin/compiler/js/compiler.pb.js'
import type {
  Dependency,
  Plugin,
} from '@go/github.com/s4wave/spacewave/bldr/project/validate/validate.pb.js'
import type { ValidatePluginRepositoryResponse } from '@s4wave/sdk/space/space.pb.js'
import { Button } from '@s4wave/web/ui/button.js'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@s4wave/web/ui/sheet.js'

import * as stylex from '@stylexjs/stylex'

import {
  fonts as themeFonts,
  colors as themeColors,
  layout as themeLayout,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: { marginTop: themeLayout['--spacing'] },
})

const stylexStyles4 = stylex.create({
  rule1: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    marginTop: themeLayout['--spacing'],
    listStyleType: 'disc',
    paddingLeft: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule4: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: { color: themeColors['--color-foreground'] },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: { overflowY: 'auto' },
  rule2: { flexDirection: 'row', justifyContent: 'flex-end' },
})

// moduleKinds labels the kinds of a plugin's JavaScript modules.
const moduleKinds: Record<number, string> = {
  [JsModuleKind.BACKEND]: 'Backend',
  [JsModuleKind.FRONTEND]: 'Frontend',
}

/**
 * PluginRepositorySheet shows what a fetched repository's checked-out commit
 * declares before anything in it runs. It renders the validation the Space
 * returned and decides nothing itself: a refused repository offers no build,
 * and a review of a commit the repository has since left must be repeated.
 */
export function PluginRepositorySheet({
  repository,
  review,
  device,
  stale,
  busy,
  onConfirm,
  onClose,
}: {
  repository: string
  review: ValidatePluginRepositoryResponse
  device: string
  stale: boolean
  busy: boolean
  onConfirm: () => void
  onClose: () => void
}) {
  // Split the validation into what the sheet lists.
  const commit = review.commit ?? ''
  const refusals = review.validation?.refusals ?? []
  const plugins = review.validation?.plugins ?? []
  const dependencies = review.validation?.dependencies ?? []
  const direct = dependencies.filter((dependency) => dependency.direct)
  const canBuild =
    refusals.length === 0 && plugins.length > 0 && !stale && !busy

  return (
    <Sheet open onOpenChange={(open) => !open && onClose()}>
      <SheetContent sx={stylexStyles.rule1}>
        <SheetHeader>
          <SheetTitle>Review {repository}</SheetTitle>
          <SheetDescription>
            Commit {commit.slice(0, 7)}. Nothing from this repository has run.
          </SheetDescription>
        </SheetHeader>
        <div sx={stylexStyles4.rule1}>
          {stale && (
            <p
              role="alert"
              sx={[nativeChildSpacingNext.s4, stylexStyles2.rule1]}
            >
              The repository has moved to another commit. Review it again.
            </p>
          )}
          {refusals.length > 0 ? (
            <section
              sx={nativeChildSpacingNext.s4}
              role="alert"
              aria-label="Refused"
            >
              <h3 sx={stylexStyles2.rule2}>
                Spacewave will not build this repository
              </h3>
              <ul sx={stylexStyles3.rule1}>
                {refusals.map((refusal) => (
                  <li sx={nativeChildSpacing.s1} key={refusal.reason}>
                    {refusal.reason}
                  </li>
                ))}
              </ul>
            </section>
          ) : (
            <>
              <section sx={nativeChildSpacingNext.s4} aria-label="Plugins">
                <h3 sx={stylexStyles2.rule3}>Plugins</h3>
                <ul sx={stylexStyles5.rule1}>
                  {plugins.map((plugin) => (
                    <PluginItem
                      sx={nativeChildSpacingNextNext.s2}
                      key={plugin.manifestId}
                      plugin={plugin}
                    />
                  ))}
                </ul>
              </section>
              <section sx={nativeChildSpacingNext.s4} aria-label="Dependencies">
                <h3 sx={stylexStyles2.rule3}>Dependencies</h3>
                <p sx={stylexStyles2.rule4}>
                  {dependencySummary(direct, dependencies.length)}
                </p>
              </section>
              <section sx={nativeChildSpacingNext.s4} aria-label="Target">
                <h3 sx={stylexStyles2.rule3}>Target</h3>
                <p sx={stylexStyles2.rule4}>
                  Builds on {device} into your developer Space, at this commit
                  only.
                </p>
              </section>
            </>
          )}
        </div>
        <SheetFooter sx={stylexStyles.rule2}>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          {refusals.length === 0 && (
            <Button size="sm" disabled={!canBuild} onClick={onConfirm}>
              Build
            </Button>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

/** PluginItem lists one declared plugin and its modules. */
function PluginItem({
  sx,
  plugin,
}: { plugin: Plugin } & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <li sx={sx}>
      <p sx={stylexStyles2.rule5}>{plugin.manifestId}</p>
      {plugin.description && (
        <p sx={stylexStyles2.rule6}>{plugin.description}</p>
      )}
      <ul sx={stylexStyles2.rule6}>
        {(plugin.modules ?? []).map((module) => (
          <li key={module.path}>
            {moduleKinds[module.kind ?? 0] ?? 'Module'}: {module.path}
          </li>
        ))}
      </ul>
    </li>
  )
}

/** dependencySummary names the direct dependencies and counts the rest. */
function dependencySummary(direct: Dependency[], total: number): string {
  // A repository without packages installs nothing.
  if (total === 0) return 'None.'

  // Name the direct dependencies, then count the packages they pull in.
  const names = direct.map(({ name, version }) => `${name}@${version}`)
  const indirect = total - direct.length
  const pinned = `each pinned by its content hash`
  if (indirect === 0) return `${names.join(', ')}, ${pinned}.`
  const more = `${indirect} more package${indirect === 1 ? '' : 's'}`
  if (names.length === 0) return `${more}, ${pinned}.`
  return `${names.join(', ')} and ${more}, ${pinned}.`
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
