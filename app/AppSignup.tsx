import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'

// AppSignup redirects to the unified login flow.
// Account creation is now handled by the LoginForm via
// SpacewaveProvider.loginOrCreateAccount.
export function AppSignup() {
  return <NavigatePath to="/login" replace />
}
