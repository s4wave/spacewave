import { useCallback, useMemo, useRef, useState } from 'react'
import {
  LuBuilding2,
  LuChevronRight,
  LuCheck,
  LuDownload,
  LuKeyRound,
  LuLayers,
  LuLink,
  LuLock,
  LuLockOpen,
  LuLogIn,
  LuShieldCheck,
} from 'react-icons/lu'
import { toast } from '@s4wave/web/ui/toaster.js'
import { isDesktop } from '@aptre/bldr'

import { useNavLinks } from '@s4wave/app/nav-links.js'
import { QuickstartCommands } from '@s4wave/app/quickstart/QuickstartCommands.js'
import {
  addTab as addShellTab,
  useShellTabs,
} from '@s4wave/app/ShellTabContext.js'
import {
  VISIBLE_QUICKSTART_OPTIONS,
  getQuickstartPath,
  type QuickstartOption,
} from '@s4wave/app/quickstart/options.js'
import { downloadPemFile } from '@s4wave/web/download.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@s4wave/web/ui/command.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { useSessionOnboardingState } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Session } from '@s4wave/sdk/session/session.js'

export interface DashboardSpace {
  id: string
  name: string
  orgId?: string
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
}: SessionDashboardProps) {
  const navigate = useNavigate()
  const isLoading = spaces === undefined
  const isEmpty = spaces?.length === 0

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
      <ShootingStars className="pointer-events-none absolute inset-0 opacity-60" />

      <div className="relative z-10 p-3">
        <DashboardNav />
      </div>

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
            onQuickstartClick={readOnly ? undefined : onQuickstartClick}
            isLoading={isLoading}
            isEmpty={isEmpty}
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
  const { tabs, activeTabId, setTabs, setActiveTabId } = useShellTabs()

  const handleDocsClick = useCallback(() => {
    const result = addShellTab(tabs, '/docs', activeTabId || undefined)
    setTabs(result.tabs)
    setActiveTabId(result.newTab.id)
  }, [tabs, activeTabId, setTabs, setActiveTabId])

  const links = [
    ...(!isDesktop ? [{ text: 'Download', onClick: nav.download }] : []),
    { text: 'Docs', onClick: handleDocsClick },
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
  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      onClick?.()
    },
    [onClick],
  )

  return (
    <a
      href="#"
      onClick={handleClick}
      className="text-foreground-alt/40 hover:text-foreground-alt px-2 py-1 text-[11px] font-medium tracking-wide uppercase transition-colors"
    >
      {text}
    </a>
  )
}

// WelcomeHeading renders a congrats message for new cloud subscribers.
function WelcomeHeading() {
  return (
    <div className="mb-6 text-center">
      <h1 className="text-foreground text-2xl font-bold tracking-wide">
        Welcome to Spacewave!
      </h1>
      <p className="text-foreground-alt mt-2 text-sm">
        Your subscription is active. Create your first space to get started.
      </p>
    </div>
  )
}

// InlineSecureAccountSection renders PEM download and PIN setup inline
// on the dashboard welcome state. It updates local session onboarding
// completion when actions are completed.
function InlineSecureAccountSection(props: {
  accountResource?: Resource<Account | null>
  session?: Session
}) {
  const onboarding = useSessionOnboardingState()

  const [password, setPassword] = useState('')
  const [downloading, setDownloading] = useState(false)
  const [pemError, setPemError] = useState<string | null>(null)

  const [lockMode, setLockMode] = useState<'auto' | 'pin'>('auto')
  const [pin, setPin] = useState('')
  const [confirmPin, setConfirmPin] = useState('')
  const [lockError, setLockError] = useState<string | null>(null)
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
    if (lockMode === 'pin') {
      if (pin.length < 4) {
        setLockError('PIN must be at least 4 digits')
        return
      }
      if (pin !== confirmPin) {
        setLockError('PINs do not match')
        return
      }
    }
    setLockError(null)
    setSavingLock(true)
    try {
      if (props.session) {
        const mode =
          lockMode === 'pin' ?
            SessionLockMode.PIN_ENCRYPTED
          : SessionLockMode.AUTO_UNLOCK
        const pinBytes =
          lockMode === 'pin' ? new TextEncoder().encode(pin) : undefined
        await props.session.setLockMode(mode, pinBytes)
      }
      onboarding.markLockComplete()
      toast.success('Lock mode set')
    } catch (err) {
      setLockError(
        err instanceof Error ? err.message : 'Failed to set lock mode',
      )
    } finally {
      setSavingLock(false)
    }
  }, [props.session, lockMode, pin, confirmPin, onboarding])

  return (
    <div className="mt-4 w-full max-w-md">
      <div className="border-ui-outline/50 rounded-lg border p-4">
        <h2 className="text-foreground mb-3 flex items-center gap-2 text-sm font-medium">
          <LuShieldCheck className="h-4 w-4" />
          Secure Your Account
        </h2>
        <div className="space-y-4">
          {onboarding.onboarding.backupComplete ?
            <div className="text-foreground-alt flex items-center gap-2 px-3 py-2 text-sm">
              <LuCheck className="text-brand h-4 w-4 shrink-0" />
              <span>Backup key downloaded</span>
            </div>
          : <div className="space-y-3">
              <div className="flex items-start gap-3">
                <div className="bg-brand/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg">
                  <LuDownload className="text-brand h-4 w-4" />
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
                <label className="text-foreground-alt mb-1.5 block text-xs select-none">
                  Account password
                </label>
                <input
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
                <LuDownload className="text-foreground h-3.5 w-3.5" />
                <span className="text-foreground text-sm">
                  {downloading ? 'Generating...' : 'Download backup .pem'}
                </span>
              </button>
              {pemError && (
                <p className="text-destructive text-xs">{pemError}</p>
              )}
            </div>
          }

          <div className="border-ui-outline/30 border-t" />

          {onboarding.onboarding.lockComplete ?
            <div className="text-foreground-alt flex items-center gap-2 px-3 py-2 text-sm">
              <LuCheck className="text-brand h-4 w-4 shrink-0" />
              <span>Session lock configured</span>
            </div>
          : <div className="space-y-3">
              <div className="flex items-start gap-3">
                <div className="bg-brand/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg">
                  <LuKeyRound className="text-brand h-4 w-4" />
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
                  selected={lockMode === 'auto'}
                  onSelect={() => setLockMode('auto')}
                  icon={<LuLockOpen className="h-4 w-4" />}
                  label="Auto-unlock"
                  description="Key stored on disk. No PIN needed on launch."
                />
                <RadioOption
                  selected={lockMode === 'pin'}
                  onSelect={() => setLockMode('pin')}
                  icon={<LuLock className="h-4 w-4" />}
                  label="PIN lock"
                  description="Key encrypted with PIN. Enter PIN on each app launch."
                />
              </div>
              {lockMode === 'pin' && (
                <div className="space-y-2">
                  <div>
                    <label className="text-foreground-alt mb-1.5 block text-xs select-none">
                      PIN
                    </label>
                    <input
                      type="password"
                      value={pin}
                      onChange={(e) => setPin(e.target.value)}
                      placeholder="Enter PIN"
                      className={cn(
                        'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
                        'focus:border-brand/50',
                      )}
                    />
                  </div>
                  <div>
                    <label className="text-foreground-alt mb-1.5 block text-xs select-none">
                      Confirm PIN
                    </label>
                    <input
                      type="password"
                      value={confirmPin}
                      onChange={(e) => setConfirmPin(e.target.value)}
                      placeholder="Confirm PIN"
                      className={cn(
                        'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
                        'focus:border-brand/50',
                        confirmPin.length > 0 &&
                          pin !== confirmPin &&
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
              {lockError && (
                <p className="text-destructive text-xs">{lockError}</p>
              )}
            </div>
          }
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
            <LuCheck className="text-brand h-4 w-4 shrink-0" />
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
          <LuBuilding2 className="h-4 w-4" />
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
            <LuBuilding2 className="text-foreground h-3.5 w-3.5" />
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
  onSpaceClick?: (space: DashboardSpace) => void
  onQuickstartClick?: (quickstartId: string) => void
  isLoading: boolean
  isEmpty: boolean
}

function DashboardCommandPalette({
  spaces,
  orgs,
  onSpaceClick,
  onQuickstartClick,
  isLoading,
  isEmpty,
}: DashboardCommandPaletteProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const ns = useStateNamespace(['session-settings'])
  const [, setDetailsPath] = useStateAtom<string>(ns, 'details-path', '/')

  const recentSpaces = useMemo(() => {
    if (!spaces || spaces.length === 0) return []
    return spaces.slice(0, 8)
  }, [spaces])

  const hasSpaces = recentSpaces.length > 0

  const handleSpaceSelect = useCallback(
    (space: DashboardSpace) => {
      onSpaceClick?.(space)
    },
    [onSpaceClick],
  )

  const handleLinkDevice = useCallback(() => {
    setDetailsPath('/link-device')
    setOpenMenu?.('account')
  }, [setDetailsPath, setOpenMenu])

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
      />
      <CommandList className="min-h-0 flex-1 overflow-y-auto bg-transparent">
        <CommandEmpty className="text-foreground-alt py-8 text-center text-sm">
          {isLoading ?
            <div className="flex items-center justify-center">
              <LoadingInline label="Loading spaces" tone="muted" size="sm" />
            </div>
          : 'No results'}
        </CommandEmpty>

        {hasSpaces && personalSpaces.length > 0 && (
          <CommandGroup
            heading={<SectionHeading label={hasOrgs ? 'Personal' : 'Spaces'} />}
            className="py-1"
          >
            {personalSpaces.map((space) => (
              <DashboardItem
                key={space.id}
                value={`space-${space.name}-${space.id}`}
                icon={LuLayers}
                iconClassName="text-brand"
                iconBgClassName="bg-brand/10 group-data-[selected=true]:bg-brand/20"
                label={space.name}
                sublabel={space.id}
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
                onLabelClick={() => navigate({ path: `./org/${org.id}` })}
              />
            }
            className="py-1"
          >
            {orgSpaces.length === 0 ?
              <div className="text-foreground-alt/40 px-2 py-3 text-center text-xs">
                No spaces yet
              </div>
            : orgSpaces.map((space) => (
                <DashboardItem
                  key={space.id}
                  value={`space-${space.name}-${space.id}`}
                  icon={LuLayers}
                  iconClassName="text-brand"
                  iconBgClassName="bg-brand/10 group-data-[selected=true]:bg-brand/20"
                  label={space.name}
                  sublabel={space.id}
                  orgName={org.displayName}
                  onSelect={() => handleSpaceSelect(space)}
                />
              ))
            }
          </CommandGroup>
        ))}

        <CommandGroup heading="Join" className="py-1">
          <DashboardItem
            value="join-link-device"
            icon={LuLink}
            iconClassName="text-foreground-alt"
            iconBgClassName="bg-foreground/5 group-data-[selected=true]:bg-foreground/10"
            label="Link a device"
            sublabel="Link to an existing device via pairing code"
            onSelect={handleLinkDevice}
          />
          <DashboardItem
            value="join-space"
            icon={LuLogIn}
            iconClassName="text-foreground-alt"
            iconBgClassName="bg-foreground/5 group-data-[selected=true]:bg-foreground/10"
            label="Join Space"
            sublabel="Join a shared space via invite code or link"
            onSelect={() => navigate({ path: './join' })}
          />
        </CommandGroup>

        <CommandGroup
          heading={isEmpty ? 'Get Started' : 'Create'}
          className="py-1"
        >
          {VISIBLE_QUICKSTART_OPTIONS.filter(
            (opt) => opt.id !== 'account' && opt.id !== 'pair',
          ).map((opt) => (
            <DashboardItem
              key={opt.id}
              value={`create-${opt.id}`}
              icon={opt.icon}
              iconClassName="text-foreground-alt"
              iconBgClassName="bg-foreground/5 group-data-[selected=true]:bg-foreground/10"
              label={opt.name}
              sublabel={opt.description}
              experimental={'experimental' in opt && !!opt.experimental}
              onSelect={() =>
                onQuickstartClick ?
                  onQuickstartClick(opt.id)
                : navigate({ path: getQuickstartPath(opt) })
              }
            />
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  )
}

function SectionHeading(props: {
  label: string
  onLabelClick?: () => void
  actionLabel?: string
  onAction?: () => void
}) {
  return (
    <span className="flex w-full items-center justify-between">
      {props.onLabelClick ?
        <button
          onClick={(e) => {
            e.stopPropagation()
            props.onLabelClick?.()
          }}
          className="hover:text-foreground cursor-pointer transition-colors"
        >
          {props.label}
        </button>
      : <span>{props.label}</span>}
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

interface DashboardItemProps {
  value: string
  icon: React.ComponentType<{ className?: string }>
  iconClassName?: string
  iconBgClassName?: string
  label: string
  sublabel?: string
  experimental?: boolean
  orgName?: string
  onSelect: () => void
}

function DashboardItem({
  value,
  icon: Icon,
  iconClassName,
  iconBgClassName,
  label,
  sublabel,
  experimental,
  orgName,
  onSelect,
}: DashboardItemProps) {
  return (
    <CommandItem
      value={value}
      className="group mx-1 flex cursor-pointer items-center gap-3 rounded-md px-3 py-2.5"
      onSelect={onSelect}
    >
      <div
        className={cn(
          'flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-colors',
          iconBgClassName,
        )}
      >
        <Icon className={cn('h-4 w-4', iconClassName)} />
      </div>
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
          {experimental && (
            <span className="bg-foreground/8 text-foreground-alt/60 shrink-0 rounded px-1 py-0.5 text-[10px] leading-none font-medium uppercase">
              Exp
            </span>
          )}
        </div>
        {sublabel && (
          <div className="text-foreground-alt/60 truncate text-xs">
            {sublabel}
          </div>
        )}
      </div>
      <LuChevronRight className="text-foreground-alt/40 group-data-[selected=true]:text-foreground-alt h-4 w-4 shrink-0 transition-colors" />
    </CommandItem>
  )
}
