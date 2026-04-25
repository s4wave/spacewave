import { useCallback, useState } from 'react'
import { LuCheck, LuCopy, LuDatabase, LuKey } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'
import { formatBytes } from '@s4wave/web/transform/TransformConfigDisplay.js'
import { EntityKeypairsSection } from './EntityKeypairsSection.js'

export interface CryptoKeysSectionProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// CryptoKeysSection wraps crypto identity display and entity keypairs into a
// single collapsible section with a compact layout.
export function CryptoKeysSection({
  open,
  onOpenChange,
}: CryptoKeysSectionProps) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'crypto', false)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { sessionInfo, providerId } = useSessionInfo(session)
  const isLocal = providerId === 'local'
  const crypto = sessionInfo?.cryptoInfo

  const [pemCopied, setPemCopied] = useState(false)
  const handleCopyPem = useCallback(() => {
    const pem = crypto?.publicKeyPem
    if (!pem) return
    void navigator.clipboard.writeText(pem)
    setPemCopied(true)
    setTimeout(() => setPemCopied(false), 2000)
  }, [crypto])

  if (!crypto && !isLocal) return null

  return (
    <CollapsibleSection
      title="Crypto & Keys"
      icon={<LuKey className="h-3.5 w-3.5" />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
    >
      <div className="space-y-3">
        {crypto && (
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              {crypto.keyType && (
                <span className="text-foreground-alt text-xs">
                  {crypto.keyType}
                </span>
              )}
              {crypto.publicKeyPem && (
                <button
                  onClick={handleCopyPem}
                  className="hover:bg-foreground/5 text-foreground-alt hover:text-foreground flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs transition-colors"
                  aria-label={pemCopied ? 'Copied!' : 'Copy public key PEM'}
                >
                  {pemCopied ?
                    <LuCheck className="h-3 w-3 text-green-500" />
                  : <LuCopy className="h-3 w-3" />}
                  <span>
                    {pemCopied ? 'Copied' : 'Export Public Key (PEM)'}
                  </span>
                </button>
              )}
            </div>
            {(crypto.spaceCount ?? 0) > 0 && (
              <div className="border-foreground/10 flex items-center gap-4 border-t pt-2">
                <div className="text-foreground-alt flex items-center gap-1 text-xs">
                  <LuDatabase className="h-3 w-3" />
                  <span>
                    {crypto.spaceCount}{' '}
                    {crypto.spaceCount === 1 ? 'space' : 'spaces'}
                  </span>
                </div>
                {(crypto.totalStorageBytes ?? 0n) > 0n && (
                  <div className="text-foreground-alt text-xs">
                    {formatBytes(crypto.totalStorageBytes ?? 0n)}
                  </div>
                )}
              </div>
            )}
          </div>
        )}
        {isLocal && (
          <>
            {crypto && <div className="border-foreground/10 border-t" />}
            <EntityKeypairsSection embedded />
          </>
        )}
      </div>
    </CollapsibleSection>
  )
}
