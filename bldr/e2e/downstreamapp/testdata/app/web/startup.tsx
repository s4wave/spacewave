import React, { useMemo } from 'react'

import { WebView } from '@aptre/bldr-react'

export default function DownstreamStartup() {
  const loading = useMemo(() => <div>Loading downstream app</div>, [])
  return <WebView loading={loading} startupProgress />
}
