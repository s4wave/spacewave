import type { GetRepoInfoResponse } from '@s4wave/sdk/git/repo.pb.js'
import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'

// gitCloneAddress is the spacewave:// address Git clones the repository
// object from: spacewave:///u/<session>/so/<space>/-/<key>, with the key raw.
// Its contract is the Git remote's parser, ParseSpacewaveURI in
// sdk/space/object-uri.go, which cleans the path and splits it on /-/ but
// never percent-decodes, so an encoded key would name another object. It is
// null without a session, Space or key, or for a key that parser would read
// back as another: an empty, ".", ".." or "-" segment, as in a key with a
// leading, trailing or doubled slash.
export function gitCloneAddress(
  sessionIndex: number,
  spaceId: string | undefined,
  objectKey: string,
): string | null {
  if (!sessionIndex || !spaceId || !objectKey) return null
  const segments = objectKey.split('/')
  if (segments.some((s) => s === '' || s === '.' || s === '..' || s === '-')) {
    return null
  }
  return `spacewave:///u/${sessionIndex}/so/${spaceId}/-/${objectKey}`
}

// GitCloneFact is the Clone fact: the address, shortened in its middle, and
// a press that copies it whole.
export function GitCloneFact({ address }: { address: string }) {
  return (
    <FactRow
      label="Clone"
      value={address}
      control={<CopyButton text={address} label="Copy clone address" />}
    />
  )
}

// GitRepoFactsProps configures GitRepoFacts.
interface GitRepoFactsProps {
  // info is the repository's info, or null while it loads.
  info: GetRepoInfoResponse | null | undefined
  // cloneAddress is the address from gitCloneAddress, or null.
  cloneAddress: string | null
}

/**
 * GitRepoFacts lists a repository's facts for its side column and Details:
 * Head, the commit HEAD resolves to with its subject under it, and Clone.
 */
export function GitRepoFacts({ info, cloneAddress }: GitRepoFactsProps) {
  // Take the subject from the last commit when it is the one HEAD names.
  const head = info?.headCommitHash
  const last = info?.lastCommit
  const subject =
    last?.hash === head ? last?.message?.split('\n', 1)[0] : undefined

  return (
    <>
      {head ? <FactRow label="Head" value={head} sub={subject} /> : null}
      {cloneAddress ? <GitCloneFact address={cloneAddress} /> : null}
    </>
  )
}

// GitCloneAction is the Clone fact under a centered state, so the clone
// address is one press away even where the viewer shows no repository.
export function GitCloneAction({ address }: { address: string | null }) {
  if (!address) return null
  return (
    <div className="mt-3 w-full max-w-xs px-3">
      <FactList dense>
        <GitCloneFact address={address} />
      </FactList>
    </div>
  )
}
