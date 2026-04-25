import { useCallback, useState } from 'react'
import { LuLock, LuLockOpen } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'

export interface SessionLockSectionProps {
  // embedded hides the section heading and outer wrapper when rendered inside
  // a parent CollapsibleSection.
  embedded?: boolean
}

// SessionLockSection displays session lock mode controls.
export function SessionLockSection({ embedded }: SessionLockSectionProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const lockStateResource = useStreamingResource(
    sessionResource,
    (session, signal) => session.watchLockState({}, signal),
    [],
  )
  const lockState = lockStateResource.value
  const loading = !lockState

  const [selectedMode, setSelectedMode] = useState<'auto' | 'pin' | null>(null)
  const [changingPin, setChangingPin] = useState(false)
  const [pin, setPin] = useState('')
  const [confirmPin, setConfirmPin] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const currentMode =
    lockState?.mode === SessionLockMode.PIN_ENCRYPTED ? 'pin' : 'auto'
  const displayMode = selectedMode ?? currentMode

  const handleSave = useCallback(async () => {
    if (displayMode === 'pin') {
      if (pin.length < 4) {
        setError('PIN must be at least 4 digits')
        return
      }
      if (pin !== confirmPin) {
        setError('PINs do not match')
        return
      }
    }
    setError(null)
    setSaving(true)
    try {
      const mode =
        displayMode === 'pin' ?
          SessionLockMode.PIN_ENCRYPTED
        : SessionLockMode.AUTO_UNLOCK
      const pinBytes =
        displayMode === 'pin' ? new TextEncoder().encode(pin) : undefined
      await session?.setLockMode(mode, pinBytes)
      setSelectedMode(null)
      setChangingPin(false)
      setPin('')
      setConfirmPin('')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to change lock mode',
      )
    } finally {
      setSaving(false)
    }
  }, [session, displayMode, pin, confirmPin])

  const hasChanges =
    changingPin || (selectedMode !== null && selectedMode !== currentMode)
  const isPinMode = displayMode === 'pin'
  const needsPin = isPinMode && hasChanges

  const content = (
    <>
      {loading && (
        <LoadingInline label="Loading lock state" tone="muted" size="sm" />
      )}
      {!loading && (
        <div className="space-y-3">
          <div className="space-y-1.5">
            <RadioOption
              selected={displayMode === 'auto'}
              onSelect={() => setSelectedMode('auto')}
              icon={<LuLockOpen className="h-4 w-4" />}
              label="Auto-unlock"
              description="Key stored on disk. No PIN needed."
            />
            <RadioOption
              selected={displayMode === 'pin'}
              onSelect={() => setSelectedMode('pin')}
              icon={<LuLock className="h-4 w-4" />}
              label="PIN lock"
              description="Key encrypted with PIN. Enter PIN on each app launch."
            />
          </div>

          {needsPin && (
            <div className="space-y-2">
              <div>
                <label className="text-foreground-alt mb-1 block text-xs select-none">
                  PIN
                </label>
                <input
                  type="password"
                  value={pin}
                  onChange={(e) => setPin(e.target.value)}
                  placeholder="Enter PIN"
                  className={cn(
                    'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-1.5 text-sm transition-colors outline-none',
                    'focus:border-brand/50',
                  )}
                />
              </div>
              <div>
                <label className="text-foreground-alt mb-1 block text-xs select-none">
                  Confirm PIN
                </label>
                <input
                  type="password"
                  value={confirmPin}
                  onChange={(e) => setConfirmPin(e.target.value)}
                  placeholder="Confirm PIN"
                  className={cn(
                    'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-1.5 text-sm transition-colors outline-none',
                    'focus:border-brand/50',
                    confirmPin.length > 0 &&
                      pin !== confirmPin &&
                      'border-destructive/50',
                  )}
                />
              </div>
            </div>
          )}

          {currentMode === 'pin' && !hasChanges && (
            <button
              onClick={() => setChangingPin(true)}
              className="text-foreground-alt hover:text-foreground text-xs transition-colors"
            >
              Change PIN
            </button>
          )}

          {error && <p className="text-destructive text-xs">{error}</p>}

          {hasChanges && (
            <div className="flex gap-2">
              <button
                onClick={() => void handleSave()}
                disabled={saving}
                className={cn(
                  'flex-1 rounded-md border py-1.5 text-sm transition-all',
                  'border-brand/30 bg-brand/10 hover:bg-brand/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                {saving ? 'Saving...' : 'Save'}
              </button>
              <button
                onClick={() => {
                  setSelectedMode(null)
                  setChangingPin(false)
                  setPin('')
                  setConfirmPin('')
                  setError(null)
                }}
                className={cn(
                  'flex-1 rounded-md border py-1.5 text-sm transition-all',
                  'border-foreground/10 hover:bg-foreground/5',
                )}
              >
                Cancel
              </button>
            </div>
          )}

          <p className="text-foreground-alt/60 text-xs">
            Changing lock mode only requires your current session. No account
            re-auth needed.
          </p>
        </div>
      )}
    </>
  )

  if (embedded) return content

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuLock className="h-3.5 w-3.5" />
          Session Lock
        </h2>
      </div>
      <InfoCard>{content}</InfoCard>
    </section>
  )
}
