import type { ReactNode } from 'react'
import { Route } from '@s4wave/web/router/router.js'
import { PlanSelectionPage } from './PlanSelectionPage.js'
import { PlanPageRouter } from './PlanPageRouter.js'
import { UpgradeRouter } from './UpgradeRouter.js'
import { UpgradeLoginPage } from './UpgradeLoginPage.js'
import { MigrateDecisionPage } from './MigrateDecisionPage.js'
import { VerifyEmailPage } from './VerifyEmailPage.js'
import { DeleteCloudAccountPage } from './DeleteCloudAccountPage.js'
import { NoActiveBillingAccountPage } from './NoActiveBillingAccountPage.js'
import { LocalSessionSetup } from '@s4wave/app/session/setup/LocalSessionSetup.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

// spacewaveSessionRoutes returns spacewave-specific route elements (plan,
// billing, upgrade, email verification) for use inside SessionContainer's
// Routes. Returns an array of Route elements that collectRoutes can flatten.
export function spacewaveSessionRoutes(
  metadata?: SessionMetadata,
): ReactNode[] {
  return [
    <Route key="plan-success" path="/plan/success">
      <PlanSelectionPage checkoutResult="success" />
    </Route>,
    <Route key="plan-cancel" path="/plan/cancel">
      <PlanSelectionPage checkoutResult="cancel" />
    </Route>,
    <Route key="plan-upgrade-login" path="/plan/upgrade/login">
      <UpgradeLoginPage />
    </Route>,
    <Route key="plan-upgrade" path="/plan/upgrade">
      <UpgradeRouter />
    </Route>,
    <Route key="plan-migrate" path="/plan/migrate">
      <MigrateDecisionPage />
    </Route>,
    <Route key="plan-free" path="/plan/free">
      <LocalSessionSetup mode="cloud" metadata={metadata} />
    </Route>,
    <Route key="plan-no-active" path="/plan/no-active">
      <NoActiveBillingAccountPage />
    </Route>,
    <Route key="plan" path="/plan">
      <PlanPageRouter />
    </Route>,
    <Route key="verify-email" path="/verify-email">
      <VerifyEmailPage />
    </Route>,
    <Route key="delete-account" path="/delete-account">
      <DeleteCloudAccountPage />
    </Route>,
  ]
}
