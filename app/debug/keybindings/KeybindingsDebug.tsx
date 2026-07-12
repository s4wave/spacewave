import { useCallback, useMemo, useState } from 'react'
import {
  LuChartNoAxesColumnIncreasing,
  LuCommand,
  LuKeyboard,
  LuLayoutList,
  LuRotateCcw,
  LuRows3,
  LuTriangleAlert,
} from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { Badge } from '@s4wave/web/ui/badge.js'
import { Button } from '@s4wave/web/ui/button.js'
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@s4wave/web/ui/tabs.js'

import { CommandFinderVariant } from './CommandFinderVariant.js'
import { FlatTableVariant } from './FlatTableVariant.js'
import { GroupedPanelsVariant } from './GroupedPanelsVariant.js'
import { KeyboardMapVariant } from './KeyboardMapVariant.js'
import { ShortcutAuditVariant } from './ShortcutAuditVariant.js'
import { useKeybindingsPrototype } from './useKeybindingsPrototype.js'

type VariantId = 'table' | 'collections' | 'finder' | 'keyboard' | 'audit'

const VARIANTS: readonly {
  id: VariantId
  label: string
  shortLabel: string
  description: string
  icon: typeof LuLayoutList
}[] = [
  {
    id: 'table',
    label: 'Inventory table',
    shortLabel: 'Table',
    description: 'Fast scanning and inline editing',
    icon: LuLayoutList,
  },
  {
    id: 'collections',
    label: 'Collections',
    shortLabel: 'Groups',
    description: 'Category and context-first browsing',
    icon: LuRows3,
  },
  {
    id: 'finder',
    label: 'Command finder',
    shortLabel: 'Finder',
    description: 'Search-first focused editing',
    icon: LuCommand,
  },
  {
    id: 'keyboard',
    label: 'Keyboard atlas',
    shortLabel: 'Keyboard',
    description: 'Spatial shortcut discovery',
    icon: LuKeyboard,
  },
  {
    id: 'audit',
    label: 'Health workbench',
    shortLabel: 'Audit',
    description: 'Conflict and coverage triage',
    icon: LuChartNoAxesColumnIncreasing,
  },
]

export function KeybindingsDebug() {
  const navigate = useNavigate()
  const [variant, setVariant] = useState<VariantId>('table')
  const prototype = useKeybindingsPrototype()

  const goBack = useCallback(() => {
    navigate({ path: '/debug' })
  }, [navigate])

  const changeVariant = useCallback((value: string) => {
    setVariant(value as VariantId)
  }, [])

  const variantProps = useMemo(
    () => ({
      commands: prototype.commands,
      conflictCommandIds: prototype.conflictCommandIds,
      customizedCommandIds: prototype.customizedCommandIds,
      setBinding: prototype.setBinding,
      resetBinding: prototype.resetBinding,
    }),
    [
      prototype.commands,
      prototype.conflictCommandIds,
      prototype.customizedCommandIds,
      prototype.resetBinding,
      prototype.setBinding,
    ],
  )

  return (
    <div className="bg-background @container flex h-full w-full flex-col overflow-hidden">
      <header className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <BackButton onClick={goBack}>Debug gallery</BackButton>
        <span className="text-foreground-alt/40 text-[10px] tracking-wider uppercase">
          Local prototype · changes reset on reload
        </span>
      </header>

      <main className="flex-1 overflow-auto px-4 py-3 @lg:px-8 @lg:py-6">
        <div className="mx-auto w-full max-w-7xl space-y-6">
          <section className="flex flex-col gap-4 @lg:flex-row @lg:items-end @lg:justify-between">
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <Badge
                  variant="outline"
                  className="border-brand/25 bg-brand/5 text-brand"
                >
                  UI exploration
                </Badge>
                <span className="text-foreground-alt/40 text-[10px] tracking-wider uppercase">
                  Keyboard shortcuts
                </span>
              </div>
              <h1 className="mt-3 text-2xl font-semibold tracking-tight">
                Five ways to make keybindings feel approachable
              </h1>
              <p className="text-foreground-alt/55 mt-2 max-w-3xl text-sm leading-relaxed">
                Each option uses the same live in-memory command set. Rebind a
                command in one view, then switch variants to see conflicts and
                customizations update everywhere.
              </p>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <Badge
                variant="secondary"
                className="bg-foreground/5 font-normal"
              >
                {prototype.commands.length} commands
              </Badge>
              <Badge
                variant={
                  prototype.conflictCount > 0 ? 'destructive' : 'outline'
                }
                className={cn(
                  'font-normal',
                  prototype.conflictCount > 0 &&
                    'bg-destructive/15 text-destructive',
                )}
              >
                <LuTriangleAlert /> {prototype.conflictCount} conflicting
              </Badge>
              <Badge
                variant="outline"
                className="border-foreground/10 font-normal"
              >
                {prototype.customizedCommandIds.size} customized
              </Badge>
              {prototype.customizedCommandIds.size > 0 ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="text-foreground-alt/60 hover:text-brand"
                  onClick={prototype.resetAllBindings}
                >
                  <LuRotateCcw /> Reset all
                </Button>
              ) : null}
            </div>
          </section>

          <Tabs value={variant} onValueChange={changeVariant} className="gap-5">
            <div className="overflow-x-auto pb-1">
              <TabsList className="bg-foreground/5 h-auto min-w-max gap-1 rounded-lg p-1">
                {VARIANTS.map((option) => {
                  const Icon = option.icon
                  return (
                    <TabsTrigger
                      key={option.id}
                      value={option.id}
                      className="data-[state=active]:border-brand/20 data-[state=active]:bg-brand/10 data-[state=active]:text-foreground border border-transparent px-3 py-2"
                    >
                      <Icon />
                      <span className="@max-lg:hidden">{option.label}</span>
                      <span className="@lg:hidden">{option.shortLabel}</span>
                    </TabsTrigger>
                  )
                })}
              </TabsList>
            </div>

            <div className="border-foreground/8 bg-foreground/2 divide-foreground/8 -mt-3 hidden grid-cols-5 divide-x overflow-hidden rounded-lg border @4xl:grid">
              {VARIANTS.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  className={cn(
                    'px-3 py-2.5 text-left transition-colors',
                    variant === option.id
                      ? 'bg-brand/5 text-foreground'
                      : 'text-foreground-alt/45 hover:text-foreground-alt/70',
                  )}
                  onClick={() => setVariant(option.id)}
                >
                  <span className="block text-xs font-medium">
                    {option.label}
                  </span>
                  <span className="mt-0.5 block text-[10px] font-normal">
                    {option.description}
                  </span>
                </button>
              ))}
            </div>

            <TabsContent value="table">
              <FlatTableVariant {...variantProps} />
            </TabsContent>
            <TabsContent value="collections">
              <GroupedPanelsVariant {...variantProps} />
            </TabsContent>
            <TabsContent value="finder">
              <CommandFinderVariant {...variantProps} />
            </TabsContent>
            <TabsContent value="keyboard">
              <KeyboardMapVariant {...variantProps} />
            </TabsContent>
            <TabsContent value="audit">
              <ShortcutAuditVariant {...variantProps} />
            </TabsContent>
          </Tabs>
        </div>
      </main>
    </div>
  )
}
