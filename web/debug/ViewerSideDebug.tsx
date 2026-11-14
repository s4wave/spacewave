import { LuServer, LuTriangleAlert } from 'react-icons/lu'

import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
import { ViewerRailGlyph, ViewerSide } from '@s4wave/web/object/ViewerSide.js'
import { useParams } from '@s4wave/web/router/router.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'

// facts are a device's facts, as the Device viewer lists them.
const facts = (
  <>
    <FactRow label="Liveness" value="Online" kind="text" control={null} />
    <FactRow label="Setup" value="Ready" kind="text" control={null} />
    <FactRow
      label="Update"
      value="Failed to apply 0.42.2"
      kind="text"
      control={null}
      sub="Checksum mismatch; 0.42.1 still runs."
    />
    <FactRow
      label="Peer ID"
      value="12D3KooWQx7Hc4Lr8VbZ3TfPa9WkEe5Yd2Hs7GuLN2mXq9"
    />
    <FactRow label="Platform" value="linux, amd64" kind="text" />
    <FactRow label="Version" value="0.42.1" kind="text" />
  </>
)

// capabilities stand in for the work beside the column.
const capabilities = ['Terminal', 'Forge worker', 'VM (v86)', 'GPU compute']

// ViewerSideDebug renders a viewer with a side column in one fixture state,
// with the viewport as the pane, for the viewer sweep: open, rail, or
// details, which opens the bar's Details popover where the pane is under
// 42rem. An unknown or missing state shows the open column.
export function ViewerSideDebug() {
  const { state } = useParams()
  return (
    <div className="bg-background-primary viewer-pane flex h-full w-full flex-col">
      <ViewerBar
        icon={<LuServer />}
        title="forge-a"
        description="linux, amd64"
        details={{
          label: 'Device',
          facts,
          narrow: true,
          defaultOpen: state === 'details',
        }}
      />
      <ViewerSide
        label="Device"
        facts={facts}
        initial={state === 'rail' ? 'rail' : 'open'}
        rail={
          <ViewerRailGlyph
            label="Update failed"
            icon={<LuTriangleAlert className="text-destructive" />}
          />
        }
      >
        <div className="h-viewer-row text-foreground-alt flex shrink-0 items-center px-2 text-xs font-medium select-none">
          Capabilities
        </div>
        {capabilities.map((name) => (
          <div
            key={name}
            className="h-viewer-row text-foreground flex shrink-0 items-center px-2 text-xs"
          >
            <span className="min-w-0 truncate">{name}</span>
          </div>
        ))}
      </ViewerSide>
    </div>
  )
}
