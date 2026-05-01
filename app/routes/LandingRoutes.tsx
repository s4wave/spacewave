import { Route } from '@s4wave/web/router/router.js'

import { Landing } from '@s4wave/app/landing/Landing.js'
import { LandingOrRedirect } from '@s4wave/app/landing/LandingOrRedirect.js'
import { LandingDrive } from '@s4wave/app/landing/LandingDrive.js'
import { LandingChat } from '@s4wave/app/landing/LandingChat.js'
import { LandingDevices } from '@s4wave/app/landing/LandingDevices.js'
import { LandingPlugins } from '@s4wave/app/landing/LandingPlugins.js'
import { LandingNotes } from '@s4wave/app/landing/LandingNotes.js'
import { LandingCli } from '@s4wave/app/landing/LandingCli.js'
import { LandingHydra } from '@s4wave/app/landing/LandingHydra.js'
import { LandingBifrost } from '@s4wave/app/landing/LandingBifrost.js'
import { LandingControllerbus } from '@s4wave/app/landing/LandingControllerbus.js'
import { Community } from '@s4wave/app/landing/Community.js'
import { TermsOfService } from '@s4wave/app/landing/TermsOfService.js'
import { PrivacyPolicy } from '@s4wave/app/landing/PrivacyPolicy.js'
import { Pricing } from '@s4wave/app/landing/Pricing.js'
import { DMCA } from '@s4wave/app/landing/DMCA.js'
import { Licenses } from '@s4wave/app/landing/Licenses.js'
import { Changelog } from '@s4wave/app/landing/Changelog.js'
import { DownloadPage } from '@s4wave/app/download/DownloadPage.js'

// LandingRoutes contains routes for landing pages and static informational pages.
export const LandingRoutes = (
  <>
    <Route path="/">
      <LandingOrRedirect />
    </Route>
    <Route path="/landing">
      <Landing />
    </Route>
    <Route path="/landing/drive">
      <LandingDrive />
    </Route>
    <Route path="/landing/chat">
      <LandingChat />
    </Route>
    <Route path="/landing/devices">
      <LandingDevices />
    </Route>
    <Route path="/landing/plugins">
      <LandingPlugins />
    </Route>
    <Route path="/landing/notes">
      <LandingNotes />
    </Route>
    <Route path="/landing/cli">
      <LandingCli />
    </Route>
    <Route path="/landing/hydra">
      <LandingHydra />
    </Route>
    <Route path="/landing/bifrost">
      <LandingBifrost />
    </Route>
    <Route path="/landing/controllerbus">
      <LandingControllerbus />
    </Route>
    <Route path="/community">
      <Community />
    </Route>
    <Route path="/tos">
      <TermsOfService />
    </Route>
    <Route path="/privacy">
      <PrivacyPolicy />
    </Route>
    <Route path="/pricing">
      <Pricing />
    </Route>
    <Route path="/dmca">
      <DMCA />
    </Route>
    <Route path="/licenses">
      <Licenses />
    </Route>
    <Route path="/changelog">
      <Changelog />
    </Route>
    <Route path="/download">
      <DownloadPage />
    </Route>
    <Route path="/download/cli">
      <DownloadPage cliOnly />
    </Route>
  </>
)
