import {
  cliMetadata as downloadCliMetadata,
  metadata as downloadMetadata,
} from '@s4wave/app/download/DownloadPage.js'
import { metadata as communityMetadata } from '@s4wave/app/landing/Community.js'
import { metadata as dmcaMetadata } from '@s4wave/app/landing/DMCA.js'
import { metadata as bifrostMetadata } from '@s4wave/app/landing/LandingBifrost.js'
import { metadata as chatMetadata } from '@s4wave/app/landing/LandingChat.js'
import { metadata as cliMetadata } from '@s4wave/app/landing/LandingCli.js'
import { metadata as controllerbusMetadata } from '@s4wave/app/landing/LandingControllerbus.js'
import { metadata as devicesMetadata } from '@s4wave/app/landing/LandingDevices.js'
import { metadata as driveMetadata } from '@s4wave/app/landing/LandingDrive.js'
import { metadata as hydraMetadata } from '@s4wave/app/landing/LandingHydra.js'
import { metadata as landingMetadata } from '@s4wave/app/landing/Landing.js'
import { metadata as licensesMetadata } from '@s4wave/app/landing/Licenses.js'
import { metadata as notesMetadata } from '@s4wave/app/landing/LandingNotes.js'
import { metadata as pluginsMetadata } from '@s4wave/app/landing/LandingPlugins.js'
import { metadata as pricingMetadata } from '@s4wave/app/landing/Pricing.js'
import { metadata as privacyMetadata } from '@s4wave/app/landing/PrivacyPolicy.js'
import { metadata as tosMetadata } from '@s4wave/app/landing/TermsOfService.js'
import { PUBLIC_QUICKSTART_OPTIONS } from '@s4wave/app/quickstart/options.js'
import { buildQuickstartMetadata } from '@s4wave/app/quickstart/QuickstartLoading.js'

export interface PageMetadata {
  title: string
  description: string
  canonicalPath?: string
  ogImage?: string
  ogType?: string
  twitterCard?: string
  jsonLd?: object
}

// PAGE_METADATA maps prerendered pathnames to statically imported metadata.
export const PAGE_METADATA: Record<string, PageMetadata> = {
  '/landing': landingMetadata,
  '/landing/drive': driveMetadata,
  '/landing/chat': chatMetadata,
  '/landing/devices': devicesMetadata,
  '/landing/plugins': pluginsMetadata,
  '/landing/notes': notesMetadata,
  '/landing/cli': cliMetadata,
  '/landing/hydra': hydraMetadata,
  '/landing/bifrost': bifrostMetadata,
  '/landing/controllerbus': controllerbusMetadata,
  '/community': communityMetadata,
  '/tos': tosMetadata,
  '/privacy': privacyMetadata,
  '/pricing': pricingMetadata,
  '/dmca': dmcaMetadata,
  '/licenses': licensesMetadata,
  '/download': downloadMetadata,
  '/download/cli': downloadCliMetadata,
}

for (const opt of PUBLIC_QUICKSTART_OPTIONS) {
  PAGE_METADATA[`/quickstart/${opt.id}`] = buildQuickstartMetadata(opt)
}

export function getMetadata(path: string): PageMetadata {
  return PAGE_METADATA[path] ?? { title: 'Spacewave', description: '' }
}
