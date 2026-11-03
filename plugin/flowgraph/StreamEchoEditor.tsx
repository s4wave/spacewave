import * as stylex from '@stylexjs/stylex'
import { useId } from 'react'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { Input } from '@s4wave/web/ui/input.js'
import { Label } from '@s4wave/web/ui/label.js'
import { Config } from '@s4wave/net/stream/echo/echo.pb.js'

// StreamEchoEditor edits the echo protocol while retaining the Device peer.
export default function StreamEchoEditor({
  sx,
  value,
  onValueChange,
}: ConfigEditorProps<Uint8Array> & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const inputId = useId()
  const config = Config.fromBinary(value)
  return (
    <div sx={sx} className="grid gap-2">
      <Label htmlFor={inputId}>Echo protocol</Label>
      <Input
        id={inputId}
        value={config.protocolId ?? ''}
        onChange={(event) =>
          onValueChange(
            Config.toBinary({ ...config, protocolId: event.target.value }),
          )
        }
      />
    </div>
  )
}
