import {
  useCallback,
  useId,
  useMemo,
  useReducer,
  useRef,
  useState,
} from 'react'
import {
  LuBuilding2,
  LuCheck,
  LuChevronDown,
  LuChevronRight,
  LuDownload,
  LuKeyRound,
  LuLayers,
  LuLink,
  LuLock,
  LuLockOpen,
  LuLogIn,
  LuShieldCheck,
} from 'react-icons/lu'
import { isDesktop } from '@aptre/bldr'
import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavLinks } from '@s4wave/app/nav-links.js'
import { QuickstartCommands } from '@s4wave/app/quickstart/QuickstartCommands.js'
import {
  getQuickstartPath,
  type QuickstartOption,
} from '@s4wave/app/quickstart/options.js'
import { useVisibleQuickstartOptions } from '@s4wave/app/quickstart/useQuickstartOptions.js'
import { useSessionOnboardingState } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { downloadPemFile } from '@s4wave/web/download.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import {
  getObjectTypeIconComponent,
  type ObjectTypeMetadataById,
} from '@s4wave/web/space/object-tree.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import { cn } from '@s4wave/web/style/utils.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { ExperimentalBadge } from '@s4wave/web/ui/ExperimentalBadge.js'
import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@s4wave/web/ui/command.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'

export interface DashboardSpace {
  id: string
  name: string
  orgId?: string
  source?: string
  objectType?: string
}

export interface DashboardOrg {
  id: string
  displayName: string
}

export interface SessionDashboardProps {
  spaces: DashboardSpace[] | undefined
  orgs?: DashboardOrg[]
  onSpaceClick?: (space: DashboardSpace) => void
  onQuickstartClick?: (quickstartId: string) => void
  readOnly?: boolean
  isCloud?: boolean
  accountResource?: Resource<Account | null>
  session?: Session
  topStatus?: string
  objectTypeMetadataById?: ObjectTypeMetadataById
}

// SessionDashboard displays the main session dashboard page.
// When the session has no spaces (empty state), shows a welcome state with
// quickstart CTAs and a "Secure Your Account" section. Cloud subscribers
// additionally see a congrats heading and a create organization section.
// Local sessions show quickstart CTAs and secure account but no congrats
// or organization section.
export function SessionDashboard({
  spaces,
  orgs,
  onSpaceClick,
  onQuickstartClick,
  readOnly,
  isCloud,
  accountResource,
  session,
  topStatus,
  objectTypeMetadataById,
}: SessionDashboardProps) {
  const navigate = useNavigate()
  const isLoading = spaces === undefined
  const isEmpty = spaces?.length === 0
  const quickstartOptions = useVisibleQuickstartOptions()

  const goToCommunity = useCallback(() => {
    navigate({ path: '/community' })
  }, [navigate])

  const handleQuickstartCommand = useCallback(
    (opt: QuickstartOption) => {
      onQuickstartClick?.(opt.id)
    },
    [onQuickstartClick],
  )

  return (
    <div className="bg-background-landing relative flex h-full w-full flex-col overflow-hidden">
      <QuickstartCommands onQuickstart={handleQuickstartCommand} />
      <div className="relative z-10 p-3">
        <DashboardNav />
      </div>

      {topStatus && (
        <div className="pointer-events-none absolute inset-x-0 top-14 z-20 flex justify-center px-4">
          <div className="border-border/60 bg-background/80 rounded-full border px-3 py-1 shadow-sm backdrop-blur">
            <LoadingInline label={topStatus} tone="muted" size="sm" />
          </div>
        </div>
      )}

      <div className="relative z-10 flex flex-1 flex-col items-center justify-center overflow-y-auto px-4 py-8">
        {isEmpty && isCloud && !readOnly && <WelcomeHeading />}
        {!isEmpty && (
          <AnimatedLogo followMouse={true} containerClassName="mb-8" />
        )}

        <div className="w-full max-w-md">
          <DashboardCommandPalette
            spaces={spaces}
            orgs={orgs}
            onSpaceClick={onSpaceClick}
            onQuickstartClick={onQuickstartClick}
            quickstartOptions={quickstartOptions}
            objectTypeMetadataById={objectTypeMetadataById}
            isLoading={isLoading}
            isEmpty={isEmpty}
            canCreate={!readOnly}
          />
        </div>

        {isEmpty && !isLoading && !readOnly && (
          <InlineSecureAccountSection
            accountResource={accountResource}
            session={session}
          />
        )}

        {isEmpty && !isLoading && isCloud && !readOnly && <CreateOrgSection />}
      </div>

      <div className="relative z-10 pb-3 text-center">
        <p className="text-foreground-alt/60 text-xs">
          local-first · encrypted ·{' '}
          <button
            onClick={goToCommunity}
            className="hover:text-foreground cursor-pointer transition-colors"
          >
            community
          </button>
        </p>
      </div>
    </div>
  )
}

function DashboardNav() {
  const nav = useNavLinks()
  const links = [
    ...(!isDesktop ? [{ text: 'Download', onClick: nav.download }] : []),
    { text: 'Docs', onClick: nav.docs },
    { text: 'Blog', onClick: nav.blog },
    { text: 'Release Notes', onClick: nav.changelog },
    { text: 'Support', onClick: nav.support },
    { text: 'Legal', onClick: nav.legal },
  ]

  return (
    <nav className="flex flex-wrap items-center">
      {links.map((link, i) => (
        <span key={link.text} className="flex items-center">
          {i > 0 && (
            <span className="text-foreground-alt/30 px-1 text-[11px]">·</span>
          )}
          <NavLink text={link.text} onClick={link.onClick} />
        </span>
      ))}
    </nav>
  )
}

function NavLink({ text, onClick }: { text: string; onClick?: () => void }) {
  const handleNavSelect = useCallback(() => onClick?.(), [onClick])

  return (
    <button
      type="button"
      onClick={handleNavSelect}
      className="text-foreground-alt/40 hover:text-foreground-alt bg-transparent px-2 py-1 text-[11px] font-medium tracking-wide uppercase transition-colors"
    >
      {text}
    </button>
  )
}

// WelcomeHeading renders a congrats message for new cloud subscribers.
function WelcomeHeading() {
  return (
    <div className="mb-6 text-center">
      <h1 className="text-foreground text-2xl font-semibold tracking-wide">
        Welcome to Spacewave!
      </h1>
      <p className="text-foreground-alt mt-2 text-sm">
        Your subscription is active. Create your first space to get started.
      </p>
    </div>
  )
}

// LockState bundles the coupled lock-mode UI inputs that transition together
// under each user action in the lock setup flow.
interface LockState {
  mode: 'auto' | 'pin'
  pin: string
  confirmPin: string
  error: string | null
}

type LockAction =
  | { type: 'set-mode'; mode: 'auto' | 'pin' }
  | { type: 'set-pin'; pin: string }
  | { type: 'set-confirm-pin'; confirmPin: string }
  | { type: 'set-error'; error: string | null }

const initialLockState: LockState = {
  mode: 'auto',
  pin: '',
  confirmPin: '',
  error: null,
}

function lockReducer(state: LockState, action: LockAction): LockState {
  switch (action.type) {
    case 'set-mode':
      return { ...state, mode: action.mode, error: null }
    case 'set-pin':
      return { ...state, pin: action.pin, error: null }
    case 'set-confirm-pin':
      return { ...state, confirmPin: action.confirmPin, error: null }
    case 'set-error':
      return { ...state, error: action.error }
  }
}

// InlineSecureAccountSection renders PEM download and PIN setup inline
// on the dashboard welcome state. It updates local session onboarding
// completion when actions are completed.
function InlineSecureAccountSection(props: {
  accountResource?: Resource<Account | null>
  session?: Session
}) {
  const passwordInputId = useId()
  const pinInputId = useId()
  const confirmPinInputId = useId()
  const onboarding = useSessionOnboardingState()

  const [password, setPassword] = useState('')
  const [downloading, setDownloading] = useState(false)
  const [pemError, setPemError] = useState<string | null>(null)

  const [lock, dispatchLock] = useReducer(lockReducer, initialLockState)
  const [savingLock, setSavingLock] = useState(false)

  const account = props.accountResource?.value

  const handleDownloadPem = useCallback(async () => {
    if (!account) return
    if (!password) {
      setPemError('Password is required to generate a backup key')
      return
    }
    setDownloading(true)
    setPemError(null)
    try {
      const resp = await account.generateBackupKey({
        credential: {
          credential: { case: 'password' as const, value: password },
        },
      })
      const pemData = resp.pemData
      if (!pemData || pemData.length === 0) {
        setPemError('No PEM data returned')
        return
      }

      downloadPemFile(pemData)

      onboarding.markBackupComplete()
      setPassword('')
      toast.success('Backup key downloaded')
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : 'Failed to generate backup key'
      setPemError(msg)
    } finally {
      setDownloading(false)
    }
  }, [account, password, onboarding])

  const handleSetLockMode = useCallback(async () => {
    if (lock.mode === 'pin') {
      if (lock.pin.length < 4) {
        dispatchLock({
          type: 'set-error',
          error: 'PIN must be at least 4 digits',
        })
        return
      }
      if (lock.pin !== lock.confirmPin) {
        dispatchLock({ type: 'set-error', error: 'PINs do not match' })
        return
      }
    }
    dispatchLock({ type: 'set-error', error: null })
    setSavingLock(true)
    try {
      if (props.session) {
        const mode =
          lock.mode === 'pin'
            ? SessionLockMode.PIN_ENCRYPTED
            : SessionLockMode.AUTO_UNLOCK
        const pinBytes =
          lock.mode === 'pin' ? new TextEncoder().encode(lock.pin) : undefined
        await props.session.setLockMode(mode, pinBytes)
      }
      onboarding.markLockComplete()
      toast.success('Lock mode set')
    } catch (err) {
      dispatchLock({
        type: 'set-error',
        error: err instanceof Error ? err.message : 'Failed to set lock mode',
      })
    } finally {
      setSavingLock(false)
    }
  }, [props.session, lock.mode, lock.pin, lock.confirmPin, onboarding])

  return (
    <div className="mt-4 w-full max-w-md">
      <div className="border-ui-outline/50 rounded-lg border p-4">
        <h2 className="text-foreground mb-3 flex items-center gap-2 text-sm font-medium">
          <LuShieldCheck className="size-4" />
          Secure Your Account
        </h2>
        <div className="space-y-4">
          {onboarding.onboarding.backupComplete ? (
            <div className="text-foreground-alt flex items-center gap-2 px-3 py-2 text-sm">
              <LuCheck className="text-brand size-4 shrink-0" />
              <span>Backup key downloaded</span>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-start gap-3">
                <div className="bg-brand/10 flex size-8 shrink-0 items-center justify-center rounded-lg">
                  <LuDownload className="text-brand size-4" />
                </div>
                <div>
                  <p className="text-foreground text-sm font-medium">
                    Download a backup key
                  </p>
                  <p className="text-foreground-alt mt-0.5 text-xs leading-relaxed">
                    A backup key gives you a second way to recover your account
                    if you lose your password.
                  </p>
                </div>
              </div>
              <div>
                <label
                  htmlFor={passwordInputId}
                  className="text-foreground-alt mb-1.5 block text-xs select-none"
                >
                  Account password
                </label>
                <input
                  id={passwordInputId}
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Enter your password"
                  className={cn(
                    'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
                    'focus:border-brand/50',
                  )}
                />
              </div>
              <button
                onClick={() => void handleDownloadPem()}
                disabled={downloading || !account || !password}
                className={cn(
                  'group w-full rounded-md border transition-all duration-300',
                  'border-brand/30 bg-brand/10 hover:bg-brand/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                  'flex h-9 items-center justify-center gap-2',
                )}
              >
                <LuDownload className="text-foreground size-3.5" />
                <span className="text-foreground text-sm">
                  {downloading ? 'Generating...' : 'Download backup .pem'}
                </span>
              </button>
              {pemError && (
                <p className="text-destructive text-xs">{pemError}</p>
              )}
            </div>
          )}

          <div className="border-ui-outline/30 border-t" />

          {onboarding.onboarding.lockComplete ? (
            <div className="text-foreground-alt flex items-center gap-2 px-3 py-2 text-sm">
              <LuCheck className="text-brand size-4 shrink-0" />
              <span>Session lock configured</span>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-start gap-3">
                <div className="bg-brand/10 flex size-8 shrink-0 items-center justify-center rounded-lg">
                  <LuKeyRound className="text-brand size-4" />
                </div>
                <div>
                  <p className="text-foreground text-sm font-medium">
                    Set up session lock
                  </p>
                  <p className="text-foreground-alt mt-0.5 text-xs leading-relaxed">
                    Controls how your session key is protected when the app is
                    closed.
                  </p>
                </div>
              </div>
              <div className="space-y-2">
                <RadioOption
                  selected={lock.mode === 'auto'}
                  onSelect={() =>
                    dispatchLock({ type: 'set-mode', mode: 'auto' })
                  }
                  icon={<LuLockOpen className="size-4" />}
                  label="Auto-unlock"
                  description="Key stored on disk. No PIN needed on launch."
                />
                <RadioOption
                  selected={lock.mode === 'pin'}
                  onSelect={() =>
                    dispatchLock({ type: 'set-mode', mode: 'pin' })
                  }
                  icon={<LuLock className="size-4" />}
                  label="PIN lock"
                  description="Key encrypted with PIN. Enter PIN on each app launch."
                />
              </div>
              {lock.mode === 'pin' && (
                <div className="space-y-2">
                  <div>
                    <label
                      htmlFor={pinInputId}
                      className="text-foreground-alt mb-1.5 block text-xs select-none"
                    >
                      PIN
                    </label>
                    <input
                      id={pinInputId}
                      type="password"
                      value={lock.pin}
                      onChange={(e) =>
                        dispatchLock({ type: 'set-pin', pin: e.target.value })
                      }
                      placeholder="Enter PIN"
                      className={cn(
                        'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
                        'focus:border-brand/50',
                      )}
                    />
                  </div>
                  <div>
                    <label
                      htmlFor={confirmPinInputId}
                      className="text-foreground-alt mb-1.5 block text-xs select-none"
                    >
                      Confirm PIN
                    </label>
                    <input
                      id={confirmPinInputId}
                      type="password"
                      value={lock.confirmPin}
                      onChange={(e) =>
                        dispatchLock({
                          type: 'set-confirm-pin',
                          confirmPin: e.target.value,
                        })
                      }
                      placeholder="Confirm PIN"
                      className={cn(
                        'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
                        'focus:border-brand/50',
                        lock.confirmPin.length > 0 &&
                          lock.pin !== lock.confirmPin &&
                          'border-destructive/50',
                      )}
                    />
                  </div>
                </div>
              )}
              <button
                onClick={() => void handleSetLockMode()}
                disabled={savingLock}
                className={cn(
                  'group w-full rounded-md border transition-all duration-300',
                  'border-brand/30 bg-brand/10 hover:bg-brand/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                  'flex h-9 items-center justify-center gap-2',
                )}
              >
                <span className="text-foreground text-sm">
                  {savingLock ? 'Saving...' : 'Set lock mode'}
                </span>
              </button>
              {lock.error && (
                <p className="text-destructive text-xs">{lock.error}</p>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// CreateOrgSection renders a prompt to create an organization.
// Only shown for cloud subscribers in the welcome state.
function CreateOrgSection() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [orgName, setOrgName] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [created, setCreated] = useState(false)

  const handleCreate = useCallback(async () => {
    if (!session || !orgName.trim()) return
    setCreating(true)
    setError(null)
    try {
      await session.spacewave.createOrganization(orgName.trim())
      setCreated(true)
      toast.success('Organization created')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to create organization',
      )
    } finally {
      setCreating(false)
    }
  }, [session, orgName])

  if (created) {
    return (
      <div className="mt-4 w-full max-w-md">
        <div className="border-ui-outline/50 rounded-lg border p-4">
          <div className="text-foreground-alt flex items-center gap-2 text-sm">
            <LuCheck className="text-brand size-4 shrink-0" />
            <span>Organization created</span>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="mt-4 w-full max-w-md">
      <div className="border-ui-outline/50 rounded-lg border p-4">
        <h2 className="text-foreground mb-3 flex items-center gap-2 text-sm font-medium">
          <LuBuilding2 className="size-4" />
          Create an Organization
        </h2>
        <p className="text-foreground-alt mb-3 text-xs leading-relaxed">
          Organizations let you collaborate with others and manage shared
          resources.
        </p>
        <div className="space-y-3">
          <input
            type="text"
            value={orgName}
            onChange={(e) => setOrgName(e.target.value)}
            placeholder="Organization name"
            className={cn(
              'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
              'focus:border-brand/50',
            )}
          />
          <button
            onClick={() => void handleCreate()}
            disabled={creating || !orgName.trim()}
            className={cn(
              'group w-full rounded-md border transition-all duration-300',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
              'flex h-9 items-center justify-center gap-2',
            )}
          >
            <LuBuilding2 className="text-foreground size-3.5" />
            <span className="text-foreground text-sm">
              {creating ? 'Creating...' : 'Create organization'}
            </span>
          </button>
          {error && <p className="text-destructive text-xs">{error}</p>}
        </div>
      </div>
    </div>
  )
}

interface DashboardCommandPaletteProps {
  spaces: DashboardSpace[] | undefined
  orgs?: DashboardOrg[]
  objectTypeMetadataById?: ObjectTypeMetadataById
  onSpaceClick?: (space: DashboardSpace) => void
  onQuickstartClick?: (quickstartId: string) => void
  quickstartOptions: QuickstartOption[]
  isLoading: boolean
  isEmpty: boolean
  canCreate: boolean
}

function DashboardCommandPalette({
  spaces,
  orgs,
  objectTypeMetadataById,
  onSpaceClick,
  onQuickstartClick,
  quickstartOptions,
  isLoading,
  isEmpty,
  canCreate,
}: DashboardCommandPaletteProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const ns = useStateNamespace(['session-settings'])
  const [, setDetailsPath] = useStateAtom<string>(ns, 'details-path', '/')

  const createQuickstartOptions = useMemo(
    () =>
      canCreate
        ? quickstartOptions.filter(
            (opt) => opt.id !== 'account' && opt.id !== 'pair',
          )
        : [],
    [canCreate, quickstartOptions],
  )

  const primaryQuickstart =
    createQuickstartOptions.find((opt) => opt.id === 'drive') ??
    createQuickstartOptions[0]
  const blankSpaceQuickstart = createQuickstartOptions.find(
    (opt) => opt.id === 'space',
  )
  const browseQuickstartOptions = createQuickstartOptions.filter(
    (opt) =>
      opt.id !== primaryQuickstart?.id && opt.id !== blankSpaceQuickstart?.id,
  )

  // Render every space so search can match all of them; the CommandList
  // scrolls past the fixed palette height instead of truncating the list.
  const recentSpaces = useMemo(() => spaces ?? [], [spaces])

  const hasSpaces = recentSpaces.length > 0
  const [query, setQuery] = useState('')
  const showScrollCue = recentSpaces.length > 2 && query.length === 0

  const handleSpaceSelect = useCallback(
    (space: DashboardSpace) => {
      onSpaceClick?.(space)
    },
    [onSpaceClick],
  )

  const handleQuickstartSelect = useCallback(
    (opt: QuickstartOption) => {
      if (onQuickstartClick) {
        onQuickstartClick(opt.id)
        return
      }
      navigate({ path: getQuickstartPath(opt) })
    },
    [navigate, onQuickstartClick],
  )

  const handleLinkDevice = useCallback(() => {
    setDetailsPath('/link-device')
    setOpenMenu?.('account')
  }, [setDetailsPath, setOpenMenu])

  const joinItems = (
    <>
      <DashboardItem
        value="join-space"
        icon={LuLogIn}
        iconTone="muted"
        label="Join Space"
        sublabel="Join a shared space via invite code or link"
        onSelect={() => navigate({ path: './join' })}
      />
      <DashboardItem
        value="join-link-device"
        icon={LuLink}
        iconTone="muted"
        label="Link a device"
        sublabel="Link to an existing device via pairing code"
        onSelect={handleLinkDevice}
      />
    </>
  )

  const { personalSpaces, orgSections } = useMemo(() => {
    const personal: DashboardSpace[] = []
    const orgMap = new Map<string, DashboardSpace[]>()
    for (const space of recentSpaces) {
      if (space.orgId) {
        const list = orgMap.get(space.orgId)
        if (list) {
          list.push(space)
        } else {
          orgMap.set(space.orgId, [space])
        }
      } else {
        personal.push(space)
      }
    }
    const sections: Array<{ org: DashboardOrg; spaces: DashboardSpace[] }> = []
    if (orgs) {
      for (const org of orgs) {
        sections.push({ org, spaces: orgMap.get(org.id) ?? [] })
      }
    }
    return { personalSpaces: personal, orgSections: sections }
  }, [recentSpaces, orgs])

  const hasOrgs = orgSections.length > 0

  return (
    <Command
      className={cn(
        'border-ui-outline bg-background-get-started/95 relative flex flex-col overflow-hidden rounded-lg border shadow-xl backdrop-blur-sm',
        hasSpaces ? 'h-[min(380px,60vh)]' : 'max-h-[min(380px,60vh)]',
      )}
    >
      <CommandInput
        ref={inputRef}
        className="border-ui-outline placeholder:text-foreground-alt/50 h-11 border-b"
        placeholder={hasSpaces ? 'Search spaces...' : 'Get started...'}
        value={query}
        onValueChange={setQuery}
      />
      <CommandList className="min-h-0 flex-1 overflow-y-auto bg-transparent">
        <CommandEmpty className="text-foreground-alt py-8 text-center text-sm">
          {isLoading ? (
            <div className="flex items-center justify-center">
              <LoadingInline label="Loading spaces" tone="muted" size="sm" />
            </div>
          ) : (
            'No results'
          )}
        </CommandEmpty>

        <CommandGroup heading="Join" className="py-1">
          {joinItems}
        </CommandGroup>
        <CommandSeparator className="bg-foreground/8" />

        {isEmpty ? (
          <>
            {primaryQuickstart && (
              <CommandGroup heading="Continue" className="py-1">
                <DashboardItem
                  value={`continue-${primaryQuickstart.id}`}
                  icon={primaryQuickstart.icon}
                  iconTone="brand"
                  label={primaryQuickstart.name}
                  sublabel={primaryQuickstart.description}
                  experimental={
                    'experimental' in primaryQuickstart &&
                    !!primaryQuickstart.experimental
                  }
                  onSelect={() => handleQuickstartSelect(primaryQuickstart)}
                />
              </CommandGroup>
            )}

            {blankSpaceQuickstart &&
              blankSpaceQuickstart.id !== primaryQuickstart?.id && (
                <CommandGroup heading="Other starts" className="py-1">
                  <DashboardItem
                    value={`create-${blankSpaceQuickstart.id}`}
                    icon={blankSpaceQuickstart.icon}
                    iconTone="muted"
                    label={blankSpaceQuickstart.name}
                    sublabel={blankSpaceQuickstart.description}
                    experimental={
                      'experimental' in blankSpaceQuickstart &&
                      !!blankSpaceQuickstart.experimental
                    }
                    onSelect={() =>
                      handleQuickstartSelect(blankSpaceQuickstart)
                    }
                  />
                </CommandGroup>
              )}

            {browseQuickstartOptions.length > 0 && (
              <CommandGroup heading="Browse templates" className="py-1">
                {browseQuickstartOptions.map((opt) => (
                  <DashboardItem
                    key={opt.id}
                    value={`create-${opt.id}`}
                    icon={opt.icon}
                    iconTone="muted"
                    label={opt.name}
                    sublabel={opt.description}
                    experimental={'experimental' in opt && !!opt.experimental}
                    onSelect={() => handleQuickstartSelect(opt)}
                  />
                ))}
              </CommandGroup>
            )}
          </>
        ) : (
          <>
            {hasSpaces && personalSpaces.length > 0 && (
              <CommandGroup
                heading={
                  <SectionHeading
                    label={hasOrgs ? 'Personal' : 'Spaces'}
                    count={personalSpaces.length}
                  />
                }
                className="py-1"
              >
                {personalSpaces.map((space) => (
                  <DashboardItem
                    key={space.id}
                    value={`space-${space.name}-${space.id}`}
                    icon={
                      space.objectType
                        ? getObjectTypeIconComponent(
                            space.objectType,
                            objectTypeMetadataById,
                            LuLayers,
                          )
                        : LuLayers
                    }
                    iconTone="brand"
                    label={space.name}
                    sublabel={getSpaceSourceLabel(space.source)}
                    identifier={space.id}
                    onSelect={() => handleSpaceSelect(space)}
                  />
                ))}
              </CommandGroup>
            )}

            {orgSections.map(({ org, spaces: orgSpaces }) => (
              <CommandGroup
                key={org.id}
                heading={
                  <SectionHeading
                    label={org.displayName}
                    count={orgSpaces.length}
                    onLabelClick={() => navigate({ path: `./org/${org.id}` })}
                  />
                }
                className="py-1"
              >
                {orgSpaces.length === 0 ? (
                  <div className="text-foreground-alt/40 px-2 py-3 text-center text-xs">
                    No spaces yet
                  </div>
                ) : (
                  orgSpaces.map((space) => (
                    <DashboardItem
                      key={space.id}
                      value={`space-${space.name}-${space.id}`}
                      icon={
                        space.objectType
                          ? getObjectTypeIconComponent(
                              space.objectType,
                              objectTypeMetadataById,
                              LuLayers,
                            )
                          : LuLayers
                      }
                      iconTone="brand"
                      label={space.name}
                      sublabel={getSpaceSourceLabel(space.source)}
                      identifier={space.id}
                      orgName={org.displayName}
                      onSelect={() => handleSpaceSelect(space)}
                    />
                  ))
                )}
              </CommandGroup>
            ))}

            {createQuickstartOptions.length > 0 && (
              <CommandGroup heading="Create" className="py-1">
                {createQuickstartOptions.map((opt) => (
                  <DashboardItem
                    key={opt.id}
                    value={`create-${opt.id}`}
                    icon={opt.icon}
                    iconTone="muted"
                    label={opt.name}
                    sublabel={opt.description}
                    experimental={'experimental' in opt && !!opt.experimental}
                    onSelect={() => handleQuickstartSelect(opt)}
                  />
                ))}
              </CommandGroup>
            )}
          </>
        )}
      </CommandList>
      {showScrollCue && (
        <div
          role="note"
          className="border-foreground/8 bg-background-card/80 text-foreground-alt/60 flex h-7 shrink-0 items-center justify-center gap-1 border-t text-[10px] select-none"
        >
          <LuChevronDown className="size-3" />
          Scroll to see all {recentSpaces.length} spaces
        </div>
      )}
    </Command>
  )
}

function getSpaceSourceLabel(source: string | undefined): string {
  switch (source) {
    case 'created':
      return 'Owned Space'
    case 'shared':
      return 'Shared Space'
    default:
      return 'Space'
  }
}

function SectionHeading(props: {
  label: string
  count?: number
  onLabelClick?: () => void
  actionLabel?: string
  onAction?: () => void
}) {
  const labelNode = (
    <>
      {props.label}
      {props.count !== undefined && props.count > 0 && (
        <span className="text-foreground-alt/40 ml-1.5 font-normal">
          {props.count}
        </span>
      )}
    </>
  )
  return (
    <span className="flex w-full items-center justify-between">
      {props.onLabelClick ? (
        <button
          onClick={(e) => {
            e.stopPropagation()
            props.onLabelClick?.()
          }}
          className="hover:text-foreground cursor-pointer transition-colors"
        >
          {labelNode}
        </button>
      ) : (
        <span>{labelNode}</span>
      )}
      {props.actionLabel && props.onAction && (
        <button
          onClick={(e) => {
            e.stopPropagation()
            props.onAction?.()
          }}
          className="text-brand/70 hover:text-brand cursor-pointer text-[10px] font-medium tracking-normal normal-case transition-colors"
        >
          + {props.actionLabel}
        </button>
      )}
    </span>
  )
}

type IconTone = 'brand' | 'muted'

const iconToneClasses: Record<IconTone, { icon: string; bg: string }> = {
  brand: {
    icon: 'text-brand',
    bg: 'bg-brand/10 group-data-[selected=true]:bg-brand/20',
  },
  muted: {
    icon: 'text-foreground-alt',
    bg: 'bg-foreground/5 group-data-[selected=true]:bg-foreground/10',
  },
}

// IconButton renders the lozenge-shaped icon container shared by every
// DashboardItem row.
function IconButton({
  icon: Icon,
  tone,
}: {
  icon: React.ComponentType<{ className?: string }>
  tone: IconTone
}) {
  const tones = iconToneClasses[tone]
  return (
    <div
      className={cn(
        'flex size-9 shrink-0 items-center justify-center rounded-lg transition-colors',
        tones.bg,
      )}
    >
      <Icon className={cn('size-4', tones.icon)} />
    </div>
  )
}

interface DashboardItemProps {
  value: string
  icon: React.ComponentType<{ className?: string }>
  iconTone: IconTone
  label: string
  sublabel?: string
  experimental?: boolean
  identifier?: string
  orgName?: string
  onSelect: () => void
}

function DashboardItem({
  value,
  icon,
  iconTone,
  label,
  sublabel,
  identifier,
  experimental,
  orgName,
  onSelect,
}: DashboardItemProps) {
  return (
    <CommandItem
      value={value}
      className="group hover:!bg-background-card/30 focus:!bg-background-card/30 focus-visible:!bg-background-card/30 data-[selected=true]:hover:!bg-background-card/30 data-[selected=true]:focus:!bg-background-card/30 data-[selected=true]:focus-visible:!bg-background-card/30 mx-1 flex cursor-pointer items-center gap-3 rounded-md bg-transparent px-3 py-2.5 data-[selected=true]:!bg-transparent"
      onSelect={onSelect}
    >
      <IconButton icon={icon} tone={iconTone} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <div className="text-foreground truncate text-sm font-medium">
            {label}
          </div>
          {orgName && (
            <span className="bg-foreground/8 text-foreground-alt/50 shrink-0 rounded px-1 py-0.5 text-[10px] leading-none font-medium">
              {orgName}
            </span>
          )}
          {experimental && <ExperimentalBadge />}
        </div>
        {(sublabel || identifier) && (
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5">
            {sublabel && (
              <div className="text-foreground-alt/60 truncate text-xs">
                {sublabel}
              </div>
            )}
            {identifier && (
              <span
                className="flex min-w-0 items-center gap-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 group-data-[selected=true]:opacity-100"
                title={identifier}
                onClick={(event) => event.stopPropagation()}
                onMouseDown={(event) => event.stopPropagation()}
              >
                <span className="text-foreground-alt/40 max-w-28 truncate font-mono text-[10px]">
                  {identifier}
                </span>
                <CopyButton
                  text={identifier}
                  label={`Copy ${label} ID`}
                  className="hover:bg-foreground/5 size-5"
                />
              </span>
            )}
          </div>
        )}
      </div>
      <LuChevronRight className="text-foreground-alt/40 group-data-[selected=true]:text-foreground-alt size-4 shrink-0 transition-colors" />
    </CommandItem>
  )
}
