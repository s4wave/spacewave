import React, { useMemo, useState } from 'react'

import { WebView } from '@aptre/bldr-react'

export default function DownstreamStartup() {
  const [interactionCount, setInteractionCount] = useState(0)
  const loading = useMemo(() => <div>Loading downstream app</div>, [])
  return (
    <>
      <button
        data-testid="downstream-startup-interaction"
        onClick={() => setInteractionCount((count) => count + 1)}
        type="button"
      >
        Startup interactions: {interactionCount}
      </button>
      <WebView loading={loading} startupProgress />
    </>
  )
}
