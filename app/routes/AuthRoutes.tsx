import { Route } from '@s4wave/web/router/router.js'

import { SessionSelector } from '@s4wave/app/session/SessionSelector.js'
import { RecoveryPage } from '@s4wave/app/session/RecoveryPage.js'
import { SSOFinishPage } from '@s4wave/app/provider/spacewave/SSOFinishPage.js'
import { SSOLinkFinishPage } from '@s4wave/app/provider/spacewave/SSOLinkFinishPage.js'
import { SSOWaitPage } from '@s4wave/app/provider/spacewave/SSOWaitPage.js'
import { SSOConfirmPage } from '@s4wave/app/provider/spacewave/SSOConfirmPage.js'
import { PasskeyPage } from '@s4wave/app/provider/spacewave/PasskeyPage.js'
import { PasskeyWaitPage } from '@s4wave/app/provider/spacewave/PasskeyWaitPage.js'
import { PasskeyConfirmPage } from '@s4wave/app/provider/spacewave/PasskeyConfirmPage.js'
import { HandoffPage } from '@s4wave/app/auth/HandoffPage.js'
import { HandoffSessionWatch } from '@s4wave/app/auth/HandoffSessionWatch.js'
import { LaunchLoginPage } from '@s4wave/app/auth/LaunchLoginPage.js'

import { AppLogin } from '../AppLogin.js'
import { AppSignup } from '../AppSignup.js'

// AuthRoutes contains routes for authentication, sessions, and account recovery.
// Pages that can carry a CLI or desktop handoff watch its auth session so an
// expired or canceled handoff shows a notice instead of a failing form.
export const AuthRoutes = (
  <>
    <Route path="/sessions">
      <SessionSelector />
    </Route>
    <Route path="/auth/sso/finish/:nonce">
      <HandoffSessionWatch>
        <SSOFinishPage />
      </HandoffSessionWatch>
    </Route>
    <Route path="/auth/sso/link/:provider/finish">
      <SSOLinkFinishPage />
    </Route>
    <Route path="/auth/sso/:provider/confirm">
      <HandoffSessionWatch>
        <SSOConfirmPage />
      </HandoffSessionWatch>
    </Route>
    <Route path="/auth/sso/:provider">
      <HandoffSessionWatch>
        <SSOWaitPage />
      </HandoffSessionWatch>
    </Route>
    <Route path="/auth/passkey/wait">
      <HandoffSessionWatch>
        <PasskeyWaitPage />
      </HandoffSessionWatch>
    </Route>
    <Route path="/auth/passkey/confirm">
      <HandoffSessionWatch>
        <PasskeyConfirmPage />
      </HandoffSessionWatch>
    </Route>
    <Route path="/auth/passkey">
      <HandoffSessionWatch>
        <PasskeyPage />
      </HandoffSessionWatch>
    </Route>
    <Route path="/auth/launch/:username">
      <LaunchLoginPage />
    </Route>
    <Route path="/login">
      <AppLogin />
    </Route>
    <Route path="/signup">
      <AppSignup />
    </Route>
    <Route path="/recover">
      <RecoveryPage />
    </Route>
    <Route path="/auth/link/:payload">
      <HandoffSessionWatch>
        <HandoffPage />
      </HandoffSessionWatch>
    </Route>
  </>
)
