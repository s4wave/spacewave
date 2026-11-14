import type { GetRepoInfoResponse } from '@s4wave/sdk/git/repo.pb.js'
import { useParams } from '@s4wave/web/router/router.js'

import { GitCloneAction, GitRepoFacts } from '@s4wave/app/git/GitRepoFacts.js'
import {
  GitViewerCenteredState,
  GitViewerFrame,
} from '@s4wave/app/git/GitViewerShell.js'

// address is a clone address as gitCloneAddress writes it.
const address =
  'spacewave:///u/1/so/01kq8n3x7w2c9vd4h6tbe5yzrm/-/repos/spacewave'

// head is the commit HEAD names, with a subject that fills a narrow column.
const head = '1f3067e4c9d2a8b7e6f5a4b3c2d1e0f9a8b7c6d5'
const info: GetRepoInfoResponse = {
  headRef: 'refs/heads/master',
  headCommitHash: head,
  lastCommit: {
    hash: head,
    message: 'web/object: show a viewer facts in a side column\n\nBody.',
  },
}

// noop stands in for the toolbar's navigation.
const noop = () => {}

// toolbarProps are a repository at master's root with no history.
const toolbarProps = {
  effectiveRef: 'master',
  refsResponse: null,
  refsLoading: false,
  onRefSelect: noop,
  currentPath: '/',
  onPathChange: noop,
  canGoBack: false,
  canGoForward: false,
  showPath: false,
}

/**
 * GitSideDebug renders the Git repository viewer's frame in one fixture
 * state, with the viewport as the pane, for the viewer sweep: open, rail,
 * details (the Details popover open below 42rem), log (no side column, Details
 * open at every width) and empty (the empty repository with its Clone row).
 * An unknown or missing state shows the open column.
 */
export function GitSideDebug() {
  // Read the fixture state from the route.
  const { state } = useParams()

  // The empty repository shows only its message and the Clone row.
  if (state === 'empty') {
    return (
      <div className="bg-background-primary viewer-pane h-full w-full">
        <GitViewerCenteredState
          title="Empty repository"
          subtitle="No commits yet"
          action={<GitCloneAction address={address} />}
        />
      </div>
    )
  }

  // The other states frame a README body beside or under the facts.
  const facts = <GitRepoFacts info={info} cloneAddress={address} />
  const log = state === 'log'
  return (
    <div className="bg-background-primary viewer-pane h-full w-full">
      <GitViewerFrame
        toolbarProps={toolbarProps}
        facts={facts}
        side={log ? undefined : state === 'rail' ? 'rail' : 'open'}
        detailsOpen={log || state === 'details'}
      >
        <div className="text-foreground min-h-0 flex-1 overflow-auto p-2 text-xs">
          README.md
        </div>
      </GitViewerFrame>
    </div>
  )
}
