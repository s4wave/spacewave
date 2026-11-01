import { LuCloudDownload } from '@s4wave/web/images/icons.js'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { formatBytes } from '@s4wave/app/system/format.js'
import type { WasiVmHandle } from '@s4wave/sdk/vm/wasivm.js'
import { WasiVmDownloads } from '@s4wave/sdk/vm/wasivm.pb.js'

// useWasiVmDownloads watches what the VM's run has downloaded for its root
// filesystem, starting again for each run. It is undefined until the run's
// first report and after the run ends.
export function useWasiVmDownloads(
  handleResource: Resource<WasiVmHandle>,
  runGeneration: bigint,
): WasiVmDownloads | undefined {
  return (
    useStreamingResource(
      handleResource,
      (handle: WasiVmHandle, signal: AbortSignal) =>
        handle.watchDownloads(signal),
      [runGeneration],
      { equals: WasiVmDownloads.equals },
    ).value ?? undefined
  )
}

// WasiVmDownloadChip shows a run downloading its root filesystem image from
// the CDN: a chip with the bytes so far and a sweeping line along the
// header's bottom edge. Both fade in at once and fade out half a second after
// the last read, so a burst of reads shows as one download. A root filesystem
// in the Space downloads nothing and shows nothing.
export function WasiVmDownloadChip({
  downloads,
}: {
  downloads: WasiVmDownloads | undefined
}) {
  if (!downloads?.remote || !downloads.bytes) return null
  const active = (downloads.inFlight ?? 0) > 0
  const bytes = formatBytes(downloads.bytes)
  return (
    <>
      <span
        data-active={active}
        aria-hidden={!active}
        title={`Downloaded ${bytes} of the VM image`}
        className="text-foreground-alt flex min-w-0 items-center gap-1 text-xs tabular-nums opacity-0 transition-opacity delay-500 duration-200 ease-out data-[active=true]:opacity-100 data-[active=true]:delay-0"
      >
        <LuCloudDownload className="text-brand size-3.5 shrink-0 animate-pulse motion-reduce:animate-none" />
        <span className="hidden @md:inline">Downloading</span>
        <span className="truncate">{bytes}</span>
      </span>
      <span role="status" className="sr-only">
        {active ? 'Downloading the VM image' : ''}
      </span>
      <div
        aria-hidden
        data-active={active}
        className="pointer-events-none absolute inset-x-0 bottom-0 h-0.5 overflow-hidden opacity-0 transition-opacity delay-500 duration-200 ease-out data-[active=true]:opacity-100 data-[active=true]:delay-0"
      >
        <div className="bg-brand animate-progress-indeterminate absolute inset-y-0 left-0 w-1/3 rounded-full motion-reduce:w-full motion-reduce:animate-none motion-reduce:opacity-50" />
      </div>
    </>
  )
}
