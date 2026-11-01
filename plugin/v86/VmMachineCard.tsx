import { useState } from 'react'
import {
  LuPlay,
  LuRotateCcw,
  LuSlidersHorizontal,
  LuTv,
} from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { VmState, type VmMount } from '@s4wave/sdk/vm/v86.pb.js'

import { VmErrorBanner } from '@s4wave/app/vm/VmErrorBanner.js'

// OverrideSlot is a boot asset a VM may take from a UnixFS object instead
// of its V86Image.
export type OverrideSlot = 'kernel' | 'rootfs' | 'bios' | 'wasm'

// overrideSlots lists the override slots with their labels in display order.
const overrideSlots: { slot: OverrideSlot; label: string }[] = [
  { slot: 'kernel', label: 'Kernel' },
  { slot: 'rootfs', label: 'Root filesystem' },
  { slot: 'bios', label: 'BIOS' },
  { slot: 'wasm', label: 'Emulator' },
]

// VmMachineCardProps describes a stopped or failed VM and its actions.
export interface VmMachineCardProps {
  name: string
  state: VmState
  errorMessage: string
  // operationError is the failure of the last request from this view.
  operationError: string
  imageKey: string
  // imageLabel names the image for display, or is "" to show imageKey.
  imageLabel: string
  memoryMb: number
  mounts: VmMount[]
  // startDisabled holds Start until the Space can bind the VM's process.
  startDisabled: boolean
  onStart: () => void
  onReset: () => void
  // overrides maps each slot to its override object key, "" for the image's
  // asset, or is null while loading.
  overrides: Record<OverrideSlot, string> | null
  unixfsKeys: string[]
  // onOverrideChange moves a slot's override from prevKey to nextKey.
  onOverrideChange: (
    slot: OverrideSlot,
    prevKey: string,
    nextKey: string,
  ) => void
}

// VmMachineCard shows a VM that is not running: what it boots and the
// action that starts it, or the error that stopped it and the reset.
export function VmMachineCard({
  name,
  state,
  errorMessage,
  operationError,
  imageKey,
  imageLabel,
  memoryMb,
  mounts,
  startDisabled,
  onStart,
  onReset,
  overrides,
  unixfsKeys,
  onOverrideChange,
}: VmMachineCardProps) {
  const failed = state === VmState.VmState_ERROR
  const [overridesOpen, setOverridesOpen] = useState(false)
  const overrideCount = overrides
    ? Object.values(overrides).filter(Boolean).length
    : 0

  return (
    <div className="flex min-h-0 flex-1 justify-center overflow-y-auto p-4 sm:p-8">
      <div className="flex w-full max-w-md flex-col gap-3">
        <section className="border-foreground/6 bg-background-card/30 flex flex-col gap-4 rounded-lg border p-4 backdrop-blur-sm">
          {/* Identity */}
          <div className="flex items-center gap-3">
            <div className="bg-foreground/5 text-foreground-alt flex size-10 shrink-0 items-center justify-center rounded-lg">
              <LuTv className="size-5" />
            </div>
            <div className="min-w-0 flex-1">
              <h2 className="text-foreground truncate text-base font-medium">
                {name}
              </h2>
              <div className="text-foreground-alt text-xs">
                x86 virtual machine
              </div>
            </div>
          </div>

          {failed && (
            <VmErrorBanner
              title="Runtime error"
              message={errorMessage || 'The VM stopped with an error.'}
            />
          )}
          {operationError && (
            <VmErrorBanner title="Operation error" message={operationError} />
          )}

          {/* Configuration */}
          <dl className="flex flex-col gap-1.5 text-xs">
            <div className="flex gap-4">
              <dt className="text-foreground-alt w-16 shrink-0">Image</dt>
              <dd className="text-foreground min-w-0 truncate" title={imageKey}>
                {imageLabel || imageKey || 'None'}
              </dd>
            </div>
            <div className="flex gap-4">
              <dt className="text-foreground-alt w-16 shrink-0">Memory</dt>
              <dd className="text-foreground">
                {memoryMb ? `${memoryMb} MB` : 'Default'}
              </dd>
            </div>
            <div className="flex gap-4">
              <dt className="text-foreground-alt w-16 shrink-0">Mounts</dt>
              <dd className="text-foreground min-w-0">
                {mounts.length === 0 ? (
                  'None'
                ) : (
                  <ul className="flex flex-col gap-0.5 font-mono">
                    {mounts.map((m) => (
                      <li
                        key={`${m.path ?? ''}:${m.objectKey ?? ''}`}
                        className="truncate"
                        title={m.objectKey}
                      >
                        {m.path || '/'}{' '}
                        <span className="text-foreground-alt">
                          {m.writable ? 'rw' : 'ro'}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </dd>
            </div>
          </dl>

          {/* Action */}
          {failed ? (
            <Button variant="destructiveOutline" onClick={onReset}>
              <LuRotateCcw />
              Reset
            </Button>
          ) : (
            <Button disabled={startDisabled} onClick={onStart}>
              <LuPlay />
              Start
            </Button>
          )}
        </section>

        <CollapsibleSection
          title="Asset overrides"
          icon={<LuSlidersHorizontal />}
          open={overridesOpen}
          onOpenChange={setOverridesOpen}
          badge={
            overrideCount > 0 && (
              <span className="text-brand text-xs">{overrideCount} set</span>
            )
          }
        >
          {!overrides ? (
            <div className="text-foreground-alt text-xs">Loading</div>
          ) : (
            <div className="flex flex-col gap-2">
              <p className="text-foreground-alt text-xs">
                Boot from a file in this Space instead of the image's asset.
              </p>
              {overrideSlots.map(({ slot, label }) => (
                <label key={slot} className="flex items-center gap-2 text-xs">
                  <span className="text-foreground w-28 shrink-0">{label}</span>
                  <select
                    value={overrides[slot]}
                    onChange={(e) =>
                      onOverrideChange(slot, overrides[slot], e.target.value)
                    }
                    className="border-foreground/10 bg-background/40 text-foreground focus-visible:ring-ring h-7 min-w-0 flex-1 rounded-md border px-2 font-mono text-xs focus-visible:ring-1 focus-visible:outline-none"
                  >
                    <option value="">From image</option>
                    {overrides[slot] &&
                      !unixfsKeys.includes(overrides[slot]) && (
                        <option value={overrides[slot]}>
                          {overrides[slot]}
                        </option>
                      )}
                    {unixfsKeys.map((key) => (
                      <option key={key} value={key}>
                        {key}
                      </option>
                    ))}
                  </select>
                </label>
              ))}
            </div>
          )}
        </CollapsibleSection>
      </div>
    </div>
  )
}
