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
      <SheetContent className="overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Review {repository}</SheetTitle>
          <SheetDescription>
            Commit {commit.slice(0, 7)}. Nothing from this repository has run.
          </SheetDescription>
        </SheetHeader>
        <div className="space-y-4 px-4 text-sm">
          {stale && (
            <p role="alert" className="text-destructive text-xs">
              The repository has moved to another commit. Review it again.
            </p>
          )}
          {refusals.length > 0 ? (
            <section role="alert" aria-label="Refused">
              <h3 className="text-destructive font-medium">
                Spacewave will not build this repository
              </h3>
              <ul className="text-foreground-alt mt-1 list-disc space-y-1 pl-4 text-xs">
                {refusals.map((refusal) => (
                  <li key={refusal.reason}>{refusal.reason}</li>
                ))}
              </ul>
            </section>
          ) : (
            <>
              <section aria-label="Plugins">
                <h3 className="font-medium">Plugins</h3>
                <ul className="mt-1 space-y-2">
                  {plugins.map((plugin) => (
                    <PluginItem key={plugin.manifestId} plugin={plugin} />
                  ))}
                </ul>
              </section>
              <section aria-label="Dependencies">
                <h3 className="font-medium">Dependencies</h3>
                <p className="text-foreground-alt mt-1 text-xs">
                  {dependencySummary(direct, dependencies.length)}
                </p>
              </section>
              <section aria-label="Target">
                <h3 className="font-medium">Target</h3>
                <p className="text-foreground-alt mt-1 text-xs">
                  Builds on {device} into your developer Space, at this commit
                  only.
                </p>
              </section>
            </>
          )}
        </div>
        <SheetFooter className="flex-row justify-end">
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
function PluginItem({ plugin }: { plugin: Plugin }) {
  return (
    <li>
      <p className="text-foreground">{plugin.manifestId}</p>
      {plugin.description && (
        <p className="text-foreground-alt text-xs">{plugin.description}</p>
      )}
      <ul className="text-foreground-alt text-xs">
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
