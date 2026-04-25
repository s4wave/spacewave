import type { FC } from 'react'
import { Landing } from '@s4wave/app/landing/Landing.js'
import { Community } from '@s4wave/app/landing/Community.js'
import { TermsOfService } from '@s4wave/app/landing/TermsOfService.js'
import { PrivacyPolicy } from '@s4wave/app/landing/PrivacyPolicy.js'
import { Pricing } from '@s4wave/app/landing/Pricing.js'
import { DMCA } from '@s4wave/app/landing/DMCA.js'
import { Licenses } from '@s4wave/app/landing/Licenses.js'
import { DownloadPage } from '@s4wave/app/download/DownloadPage.js'
import { LandingDrive } from '@s4wave/app/landing/LandingDrive.js'
import { LandingChat } from '@s4wave/app/landing/LandingChat.js'
import { LandingDevices } from '@s4wave/app/landing/LandingDevices.js'
import { LandingPlugins } from '@s4wave/app/landing/LandingPlugins.js'
import { LandingNotes } from '@s4wave/app/landing/LandingNotes.js'
import { LandingCli } from '@s4wave/app/landing/LandingCli.js'
import { LandingHydra } from '@s4wave/app/landing/LandingHydra.js'
import { LandingBifrost } from '@s4wave/app/landing/LandingBifrost.js'
import { LandingControllerbus } from '@s4wave/app/landing/LandingControllerbus.js'
import { PUBLIC_QUICKSTART_OPTIONS } from '@s4wave/app/quickstart/options.js'
import type { QuickstartOption } from '@s4wave/app/quickstart/options.js'
import { QuickstartLoading } from '@s4wave/app/quickstart/QuickstartLoading.js'

// STATIC_PAGES maps pathnames to components for prerender and Startup.
// Paths must match STATIC_ROUTES in web/router/static-routes.ts.
// '/' is omitted here -- it uses a special dual template (see Phase 4.4).
export const STATIC_PAGES: Array<{ path: string; component: FC }> = [
  { path: '/landing', component: Landing },
  { path: '/landing/drive', component: LandingDrive },
  { path: '/landing/chat', component: LandingChat },
  { path: '/landing/devices', component: LandingDevices },
  { path: '/landing/plugins', component: LandingPlugins },
  { path: '/landing/notes', component: LandingNotes },
  { path: '/landing/cli', component: LandingCli },
  { path: '/landing/hydra', component: LandingHydra },
  { path: '/landing/bifrost', component: LandingBifrost },
  { path: '/landing/controllerbus', component: LandingControllerbus },
  { path: '/community', component: Community },
  { path: '/tos', component: TermsOfService },
  { path: '/privacy', component: PrivacyPolicy },
  { path: '/pricing', component: Pricing },
  { path: '/dmca', component: DMCA },
  { path: '/licenses', component: Licenses },
  { path: '/download', component: DownloadPage },
]

// buildQuickstartStaticPages maps public quickstarts to their static loading
// pages.
export function buildQuickstartStaticPages(
  options: readonly QuickstartOption[],
): Array<{ path: string; component: FC }> {
  return options.map((opt) => ({
    path: `/quickstart/${opt.id}`,
    component: QuickstartLoading,
  }))
}

STATIC_PAGES.push(...buildQuickstartStaticPages(PUBLIC_QUICKSTART_OPTIONS))

// getStaticPageComponent returns the component for a static page pathname.
export function getStaticPageComponent(pathname: string): FC | null {
  const page = STATIC_PAGES.find((p) => p.path === pathname)
  return page?.component ?? null
}
