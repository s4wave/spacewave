---
title: Authentication Flows
section: internals
order: 3
summary: Login flow internals, web vs desktop SSO ownership, and error semantics.
---

## Overview

Spacewave supports multiple authentication methods: password, passkey
(WebAuthn), and SSO (Google, GitHub). All authentication goes through the Go
WASM runtime via in-process starpc RPCs. The TypeScript frontend never performs
cryptographic operations or direct HTTP requests to the cloud. Instead, it
calls typed RPC methods on the SDK's `SpacewaveProvider` class.

## Authentication Routes

The auth routes are defined in `app/routes/AuthRoutes.tsx`:

| Route | Component | Purpose |
|-------|-----------|---------|
| `/login` | AppLogin | Password and SSO login form |
| `/signup` | AppSignup | Account creation |
| `/auth/passkey` | PasskeyPage | Web passkey login and signup |
| `/auth/passkey/wait` | PasskeyWaitPage | Native desktop passkey wait/login flow |
| `/auth/passkey/confirm` | PasskeyConfirmPage | Native desktop passkey account creation |
| `/auth/sso/finish/:nonce` | SSOFinishPage | Web SSO callback handler |
| `/auth/sso/desktop` | DesktopSSOCreatePage | Native desktop SSO account creation |
| `/auth/link/:payload` | HandoffPage | Generic browser handoff for desktop or CLI clients |
| `/sessions` | SessionSelector | Switch between sessions |
| `/recover` | RecoveryPage | Account recovery |

## Password Login

The `useSpacewaveAuth` hook in `app/provider/spacewave/useSpacewaveAuth.tsx` orchestrates password-based login. It looks up the `spacewave` provider via `root.lookupProvider()`, creates a `SpacewaveProvider` SDK instance, and calls `loginAccount()`:

```typescript
const resp = await sw.loginAccount({
  entityId: username,
  turnstileToken,
  credential: { value: { password }, case: 'password' },
})
```

The response contains one of three result cases:

- `session` - Login succeeded, includes the session index for navigation.
- `isNewAccount` - The email is not registered, prompt to create an account.
- `errorCode` - Login failed with a specific error code (wrong password, rate limited, etc.).

## Web SSO Login

SSO login redirects the browser to the cloud SSO endpoint. The `handleSignInWithSSO` callback reads the `ssoBaseUrl` from the pre-auth cloud provider config and redirects:

```typescript
window.location.href = `${ssoBaseUrl}/${provider}?origin=${origin}`
```

After the OAuth flow completes, the cloud redirects back to `/auth/sso/finish/:nonce`. The `SSOFinishPage` component exchanges the nonce for a session via the Go runtime.

## Desktop SSO Login

Desktop SSO uses `StartDesktopSSO`. The native app asks the cloud to create an
auth session, opens a relay WebSocket with the returned ticket, and opens the
system browser directly to the returned `open_url`.

After the OAuth flow completes, the cloud callback pushes the result back to the
waiting desktop auth session. The native app handles the rest locally:

- Linked account: log in immediately with the returned entity key and mount the
  session.
- New account: navigate to `/auth/sso/desktop` and complete username setup in
  native UI through `ConfirmDesktopSSO`.

The browser handles the OAuth provider interaction. The native app owns desktop
session completion.

## SessionDetails SSO Link

Linking an OAuth provider to an existing session uses the `SSOLinkDialog` in
the SessionDashboard auth-methods section. The flow branches on platform so
the two surfaces can evolve independently.

- Web: the dialog calls `startSSOPopupFlow` to open the provider page in a
  `window.open` popup with `mode=link`. The popup posts the `{provider, code}`
  result back to the originating window via a `BroadcastChannel`. The dialog
  then feeds the code into `AuthConfirmDialog` which calls
  `acc.linkSSO({provider, code, redirectUri, credential})`.
- Desktop: the dialog calls `session.spacewave.startDesktopSSOLink({
  ssoProvider })`. The native handler issues a session-authenticated
  `POST /api/auth/sso/link/start` to the cloud, which returns the provider
  authorize URL and a WebSocket relay ticket. The native app opens the system
  browser and waits on the auth-session relay for a `DesktopSSOLinkResult`
  payload. The returned code feeds into the same `AuthConfirmDialog` +
  `acc.linkSSO` completion path.

Desktop linking is native-owned: the renderer never opens a popup, never
reads `window.location.origin`, and never sends a desktop shell origin to
the cloud. The cloud derives the authorize-URL origin from its own
`getAppOrigin` configuration for the desktop-link mode. Web linking remains
browser-owned and keeps the popup + BroadcastChannel path.

## Passkey Login

Web passkey stays browser-owned. `/auth/passkey` runs the username-first
WebAuthn ceremony in the browser, calls pre-auth provider RPCs for username
check, auth options, registration challenge, and signup confirmation, then logs
in locally with the generated entity key.

Desktop passkey is split across browser and native surfaces:

- `StartDesktopPasskey` creates the auth session, opens the returned browser
  URL rooted at `account.spacewave.app`, and waits on the auth-session relay.
- `account.spacewave.app/passkey/login` handles the browser ceremony.
  Existing-account assertions are verified in the browser and relayed back to
  native alpha. New-account browser registration relays username plus
  registration artifacts back to native alpha.
- `/auth/passkey/wait` and `/auth/passkey/confirm` finish the linked-login or
  account-creation path natively.

## Browser Handoff

The `/auth/link/:payload` route handles browser-delegated session handoff from
the browser to the desktop app. The payload contains an encrypted session
reference. The `HandoffPage` decrypts it through the Go runtime and mounts the
session. This route serves generic continue-in-browser and CLI/browser-login
flows.

## Cloud Provider Config

The `useCloudProviderConfig` hook fetches pre-authentication configuration from
the Spacewave provider. This includes the SSO base URL, Turnstile site key, and
per-provider SSO readiness flags so the client can hide SSO methods that are
not configured server-side. The config is fetched once via a
`GetCloudProviderConfig` RPC and does not require an active session.

## Error Handling

Authentication errors are returned as typed error codes in the login response, not as thrown exceptions. The UI maps these codes to user-facing messages. Common codes include invalid credentials, rate limiting, and account locked states. Network-level errors (provider unavailable, WASM not ready) are surfaced through the `Resource` loading/error state.

## Current Boundary

- Web passkey is browser-owned through `/auth/passkey`.
- Desktop passkey uses `account.spacewave.app/passkey/login` for the browser
  ceremony and `/auth/passkey/wait` plus `/auth/passkey/confirm` for native
  completion.
- Web SSO is browser-owned after redirect and finishes through
  `/auth/sso/finish/:nonce`.
- Desktop SSO is native-owned after OAuth and finishes through the auth-session
  relay plus `/auth/sso/desktop` when account creation is needed.
- Desktop SessionDetails SSO link is native-owned through
  `StartDesktopSSOLink` + the auth-session relay, then finishes through the
  existing `AuthConfirmDialog` + `acc.linkSSO` path. Web SessionDetails SSO
  link remains browser-owned through the popup + BroadcastChannel path.
- Generic browser handoff serves non-passkey, non-SSO desktop/CLI auth flows.

## Next Steps

- [State Atoms and Persistence](/docs/developers/internals/state-atoms-and-persistence) for how session state is persisted across reloads.
- [Quickstart Seeding and Routing](/docs/developers/internals/quickstart-seeding-and-routing) for the post-auth space creation flow.
