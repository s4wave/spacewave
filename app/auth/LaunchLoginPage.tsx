import React from 'react'

import { useParams } from '@s4wave/web/router/router.js'

import { AppLogin } from '../AppLogin.js'

// LaunchLoginPage pre-fills the login screen for a known cloud username and
// redirects into an already-mounted matching session when possible.
export function LaunchLoginPage(): React.ReactElement {
  const params = useParams()
  const username = params.username ?? ''
  return <AppLogin initialUsername={username} launchUsername={username} />
}
