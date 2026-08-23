import { createElement, type FC } from 'react'
import {
  Landing,
  metadata as landingMetadata,
} from '@s4wave/app/landing/Landing.js'
import {
  Community,
  metadata as communityMetadata,
} from '@s4wave/app/landing/Community.js'
import {
  TermsOfService,
  metadata as tosMetadata,
} from '@s4wave/app/landing/TermsOfService.js'
import {
  PrivacyPolicy,
  metadata as privacyMetadata,
} from '@s4wave/app/landing/PrivacyPolicy.js'
import {
  Pricing,
  metadata as pricingMetadata,
} from '@s4wave/app/landing/Pricing.js'
import { DMCA, metadata as dmcaMetadata } from '@s4wave/app/landing/DMCA.js'
import {
  Licenses,
  metadata as licensesMetadata,
} from '@s4wave/app/landing/Licenses.js'
import {
  DownloadPage,
  cliMetadata as downloadCliMetadata,
  metadata as downloadMetadata,
} from '@s4wave/app/download/DownloadPage.js'
import {
  LandingDrive,
  metadata as driveMetadata,
} from '@s4wave/app/landing/LandingDrive.js'
import {
  LandingChat,
  metadata as chatMetadata,
} from '@s4wave/app/landing/LandingChat.js'
import {
  LandingDevices,
  metadata as devicesMetadata,
} from '@s4wave/app/landing/LandingDevices.js'
import {
  LandingPlugins,
  metadata as pluginsMetadata,
} from '@s4wave/app/landing/LandingPlugins.js'
import {
  LandingNotes,
  metadata as notesMetadata,
} from '@s4wave/app/landing/LandingNotes.js'
import {
  LandingCli,
  metadata as cliMetadata,
} from '@s4wave/app/landing/LandingCli.js'
import {
  LandingHydra,
  metadata as hydraMetadata,
} from '@s4wave/app/landing/LandingHydra.js'
import {
  LandingBifrost,
  metadata as bifrostMetadata,
} from '@s4wave/app/landing/LandingBifrost.js'
import {
  LandingControllerbus,
  metadata as controllerbusMetadata,
} from '@s4wave/app/landing/LandingControllerbus.js'
import { PUBLIC_QUICKSTART_OPTIONS } from '@s4wave/app/quickstart/options.js'
import type { QuickstartOption } from '@s4wave/app/quickstart/options.js'
import {
  QuickstartLoading,
  buildQuickstartMetadata,
} from '@s4wave/app/quickstart/QuickstartLoading.js'
import type { PageMetadata } from './metadata.js'

// StaticRoute is one prerendered public page: path, component, and SEO
// metadata live together so the inventory cannot drift.
export interface StaticRoute {
  path: string
  component: FC
  metadata: PageMetadata
}

function DownloadCliPage() {
  return createElement(DownloadPage, { cliOnly: true })
}

// STATIC_ROUTES maps pathnames to components for prerender and Startup.
// Paths must match STATIC_ROUTES in web/router/static-routes.ts.
// '/' is omitted here -- the root path uses a dual landing + loading template.
export const STATIC_ROUTES: StaticRoute[] = [
  { path: '/landing', component: Landing, metadata: landingMetadata },
  { path: '/landing/drive', component: LandingDrive, metadata: driveMetadata },
  { path: '/landing/chat', component: LandingChat, metadata: chatMetadata },
  {
    path: '/landing/devices',
    component: LandingDevices,
    metadata: devicesMetadata,
  },
  {
    path: '/landing/plugins',
    component: LandingPlugins,
    metadata: pluginsMetadata,
  },
  { path: '/landing/notes', component: LandingNotes, metadata: notesMetadata },
  { path: '/landing/cli', component: LandingCli, metadata: cliMetadata },
  { path: '/landing/hydra', component: LandingHydra, metadata: hydraMetadata },
  {
    path: '/landing/bifrost',
    component: LandingBifrost,
    metadata: bifrostMetadata,
  },
  {
    path: '/landing/controllerbus',
    component: LandingControllerbus,
    metadata: controllerbusMetadata,
  },
  { path: '/community', component: Community, metadata: communityMetadata },
  { path: '/tos', component: TermsOfService, metadata: tosMetadata },
  { path: '/privacy', component: PrivacyPolicy, metadata: privacyMetadata },
  { path: '/pricing', component: Pricing, metadata: pricingMetadata },
  { path: '/dmca', component: DMCA, metadata: dmcaMetadata },
  { path: '/licenses', component: Licenses, metadata: licensesMetadata },
  { path: '/download', component: DownloadPage, metadata: downloadMetadata },
  {
    path: '/download/cli',
    component: DownloadCliPage,
    metadata: downloadCliMetadata,
  },
]

// buildQuickstartStaticPages maps public quickstarts to their static loading
// pages.
export function buildQuickstartStaticPages(
  options: readonly QuickstartOption[],
): StaticRoute[] {
  return options.map((opt) => ({
    path: `/quickstart/${opt.id}`,
    component: QuickstartLoading,
    metadata: buildQuickstartMetadata(opt),
  }))
}

STATIC_ROUTES.push(...buildQuickstartStaticPages(PUBLIC_QUICKSTART_OPTIONS))

// getStaticPageComponent returns the component for a static page pathname.
export function getStaticPageComponent(pathname: string): FC | null {
  const page = STATIC_ROUTES.find((p) => p.path === pathname)
  if (pathname.startsWith('/quickstart/')) return QuickstartLoading
  return page?.component ?? null
}
