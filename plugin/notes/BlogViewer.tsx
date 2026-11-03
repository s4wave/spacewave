import { useCallback, useMemo, useState } from 'react'

import { LuBookOpen, LuPenLine } from '@s4wave/web/images/icons.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { ViewerStatusShell } from '@s4wave/web/object/ViewerStatusShell.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
} from '@s4wave/web/hooks/useUnixFSHandle.js'

import { Blog } from './proto/blog.pb.js'
import { BlogTypeID } from './sdk/blog.js'
import { useBlogPosts } from './blog/useBlogPosts.js'
import { useAuthorRegistry } from './blog/authors.js'
import { BlogReadingView } from './blog/BlogReadingView.js'
import type { BlogPostData } from './blog/types.js'
import { useWorldObjectMessageState } from './useWorldObjectMessageState.js'
import { createTextFile } from './write-file.js'

import NoteList from './NoteList.js'
import NoteContentView from './NoteContentView.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-leading': '1',
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 1,
  },
  rule2: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule3: {
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule6: {
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: { color: themeColors['--color-brand'] },
  rule8: { color: themeColors['--color-foreground-alt'] },
  rule9: { flex: 1, minHeight: 0 },
  rule10: { overflow: 'hidden', display: 'flex', height: '100%' },
  rule11: {
    borderColor: themeColors['--color-border'],
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    minWidth: `calc(${themeLayout['--spacing']} * 62.5)`,
    width: `calc(${themeLayout['--spacing']} * 62.5)`,
  },
  rule12: { flex: 1, minWidth: 0 },
  rule13: {
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// BlogViewer is the viewer for Notes Blog objects.
// Supports reading mode (blog reading view) and editing mode (NoteList + NoteContentView).
function BlogViewer({ objectInfo, worldState }: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const ns = useStateNamespace(['blog'])

  const { state, sources } = useWorldObjectMessageState(
    worldState,
    objectKey,
    Blog.fromBinary,
  )
  useObjectName(state.value?.name)

  // Parse the first source to get the UnixFS object key and subpath.
  const firstSource = sources[0]
  const firstSourceRef = firstSource?.ref
  const parsed = useMemo(() => {
    if (!firstSourceRef) return null
    return parseObjectUri(firstSourceRef)
  }, [firstSourceRef])

  const sourceObjectKey = parsed?.objectKey ?? ''
  const sourceSubpath = parsed?.path ?? ''

  // Access UnixFS root for reading blog posts.
  const rootHandle = useUnixFSRootHandle(worldState, sourceObjectKey)
  const pathHandle = useUnixFSHandle(rootHandle, sourceSubpath)
  const entriesResource = useUnixFSHandleEntries(pathHandle, {
    enabled: !!sourceObjectKey,
  })

  const [contentVersion, setContentVersion] = useState(0)

  // Parse all md files into blog post data for reading view.
  const blogPostsResource = useBlogPosts(
    pathHandle,
    entriesResource,
    contentVersion,
  )
  const blogPosts = useMemo(
    () => blogPostsResource.value ?? [],
    [blogPostsResource.value],
  )

  // Load author registry from the source's authors.yaml file.
  const authorRegistryPath = state.value?.authorRegistryPath ?? ''
  const authorRegistryResource = useAuthorRegistry(
    pathHandle,
    authorRegistryPath,
  )
  const authorRegistry = useMemo(
    () => authorRegistryResource.value ?? {},
    [authorRegistryResource.value],
  )

  // Persisted state: mode toggle and selected post.
  const [mode, setMode] = useStateAtom<'reading' | 'editing'>(
    ns,
    'mode',
    'reading',
  )
  const [selectedPostName, setSelectedPostName] = useStateAtom<string>(
    ns,
    'selectedPost',
    '',
  )
  const [editing, setEditing] = useStateAtom<boolean>(ns, 'editing', false)

  // Find selected post data for reading view.
  const selectedPost = useMemo(
    () => blogPosts.find((p) => p.name === selectedPostName) ?? null,
    [blogPosts, selectedPostName],
  )

  const handleSelectPostReading = useCallback(
    (post: BlogPostData | null) => {
      setSelectedPostName(post?.name ?? '')
    },
    [setSelectedPostName],
  )

  const handleSelectPostEditing = useCallback(
    (path: string) => {
      setSelectedPostName(path)
      setEditing(false)
    },
    [setSelectedPostName, setEditing],
  )

  const handleToggleEdit = useCallback(() => {
    setEditing((prev) => !prev)
  }, [setEditing])

  const handleSetReading = useCallback(() => {
    setMode('reading')
  }, [setMode])

  const handleSetEditing = useCallback(() => {
    setMode('editing')
  }, [setMode])

  // Build a set of draft post filenames for the badge indicator.
  const draftNames = useMemo(() => {
    const names = new Set<string>()
    for (const post of blogPosts) {
      if (post.draft) names.add(post.name)
    }
    return names
  }, [blogPosts])

  // Render a "Draft" badge next to draft post entries in the file list.
  const renderDraftBadge = useCallback(
    (name: string) => {
      if (!draftNames.has(name)) return null
      return <span sx={stylexStyles2.rule1}>Draft</span>
    },
    [draftNames],
  )

  // Create a new blog post with frontmatter template.
  const handleCreateBlogPost = useCallback(async () => {
    const handle = pathHandle.value
    if (!handle) return

    const existing = new Set(
      (entriesResource.value ?? []).flatMap((e) =>
        !e.isDir && e.name.endsWith('.md') ? [e.name] : [],
      ),
    )
    let name = 'new-post.md'
    let counter = 1
    while (existing.has(name)) {
      name = `new-post-${counter}.md`
      counter++
    }

    const dateStr = new Date().toISOString().slice(0, 10)
    const template =
      '---\n' +
      'title: New Post\n' +
      'date: ' +
      dateStr +
      '\n' +
      'author: \n' +
      'summary: \n' +
      'tags: []\n' +
      'draft: true\n' +
      '---\n' +
      '\n' +
      '# New Post\n' +
      '\n'

    await createTextFile(handle, name, template)
    handleSelectPostEditing(name)
  }, [pathHandle.value, entriesResource.value, handleSelectPostEditing])

  const handleCreateBlogPostClick = useCallback(() => {
    void handleCreateBlogPost()
  }, [handleCreateBlogPost])

  const handleContentSaved = useCallback(() => {
    setContentVersion((prev) => prev + 1)
  }, [])

  return (
    <ViewerStatusShell
      resource={state}
      state={state}
      loadingText="Loading blog..."
      emptyText="No sources configured for this blog"
      sources={sources}
    >
      <div sx={stylexStyles2.rule2}>
        {/* Mode toggle header */}
        <div sx={stylexStyles2.rule3}>
          <span sx={stylexStyles2.rule4}>{state.value?.name ?? 'Blog'}</span>
          <div sx={stylexStyles2.rule5}>
            <button
              type="button"
              onClick={handleSetReading}
              sx={[
                stylexStyles2.rule6,
                mode === 'reading' ? stylexStyles2.rule7 : stylexStyles2.rule8,
              ]}
              title="Reading mode"
            >
              <LuBookOpen sx={stylexStyles.rule1} />
              Read
            </button>
            <button
              type="button"
              onClick={handleSetEditing}
              sx={[
                stylexStyles2.rule6,
                mode === 'editing' ? stylexStyles2.rule7 : stylexStyles2.rule8,
              ]}
              title="Editing mode"
            >
              <LuPenLine sx={stylexStyles.rule1} />
              Edit
            </button>
          </div>
        </div>

        {/* Content area */}
        <div sx={stylexStyles2.rule9}>
          {mode === 'reading' ? (
            <BlogReadingView
              posts={blogPosts}
              selectedPost={selectedPost}
              onSelectPost={handleSelectPostReading}
              authorRegistry={authorRegistry}
            />
          ) : (
            <div sx={stylexStyles2.rule10}>
              {/* Post list sidebar */}
              <div sx={stylexStyles2.rule11}>
                <NoteList
                  source={firstSource}
                  worldState={worldState}
                  selectedNote={selectedPostName}
                  onSelectNote={handleSelectPostEditing}
                  onCreateNote={handleCreateBlogPostClick}
                  allowedFormats={['markdown']}
                  renderEntryExtra={renderDraftBadge}
                />
              </div>

              {/* Editor area */}
              <div sx={stylexStyles2.rule12}>
                {firstSource?.ref && selectedPostName ? (
                  <NoteContentView
                    worldState={worldState}
                    sourceRef={firstSource.ref}
                    noteName={selectedPostName}
                    editing={editing}
                    onToggleEdit={handleToggleEdit}
                    onContentSaved={handleContentSaved}
                  />
                ) : (
                  <div sx={stylexStyles2.rule13}>Select a post to edit</div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </ViewerStatusShell>
  )
}

export { BlogViewer, BlogTypeID }
export default BlogViewer
