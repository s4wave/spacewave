import { useCallback, useMemo } from 'react'

import { LuBookOpen, LuPenLine } from 'react-icons/lu'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { ViewerStatusShell } from '@s4wave/web/object/ViewerStatusShell.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { cn } from '@s4wave/web/style/utils.js'

import { Blog } from './proto/blog.pb.js'
import { BlogTypeID } from './sdk/blog.js'
import { useBlogPosts } from './blog/useBlogPosts.js'
import { useAuthorRegistry } from './blog/authors.js'
import { BlogReadingView } from './blog/BlogReadingView.js'
import type { BlogPostData } from './blog/types.js'
import { useWorldObjectMessageState } from './useWorldObjectMessageState.js'

import NoteList from './NoteList.js'
import NoteContentView from './NoteContentView.js'

// BlogViewer is the viewer for spacewave-notes Blog objects.
// Supports reading mode (blog reading view) and editing mode (NoteList + NoteContentView).
function BlogViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const ns = useStateNamespace(['blog'])

  const { state, sources } = useWorldObjectMessageState(
    worldState,
    objectKey,
    Blog.fromBinary,
  )

  // Parse the first source to get the UnixFS object key and subpath.
  const firstSource = sources[0]
  const parsed = useMemo(() => {
    if (!firstSource?.ref) return null
    return parseObjectUri(firstSource.ref)
  }, [firstSource?.ref])

  const sourceObjectKey = parsed?.objectKey ?? ''
  const sourceSubpath = parsed?.path ?? ''

  // Access UnixFS root for reading blog posts.
  const rootHandle = useUnixFSRootHandle(worldState, sourceObjectKey)
  const pathHandle = useUnixFSHandle(rootHandle, sourceSubpath)
  const entriesResource = useUnixFSHandleEntries(pathHandle, {
    enabled: !!sourceObjectKey,
  })

  // Parse all md files into blog post data for reading view.
  const blogPostsResource = useBlogPosts(pathHandle, entriesResource)
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
      return (
        <span className="bg-brand/10 text-brand shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium leading-none">
          Draft
        </span>
      )
    },
    [draftNames],
  )

  // Create a new blog post with frontmatter template.
  const handleCreateBlogPost = useCallback(async () => {
    const handle = pathHandle.value
    if (!handle) return

    const existing = new Set(
      (entriesResource.value ?? [])
        .filter((e) => !e.isDir && e.name.endsWith('.md'))
        .map((e) => e.name),
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
      'date: ' + dateStr + '\n' +
      'author: \n' +
      'summary: \n' +
      'tags: []\n' +
      'draft: true\n' +
      '---\n' +
      '\n' +
      '# New Post\n' +
      '\n'

    await handle.mknod([name], MknodType.FILE)
    const child = await handle.lookup(name)
    const encoded = new TextEncoder().encode(template)
    await child.writeAt(0n, encoded)
    child.release()
    handleSelectPostEditing(name)
  }, [pathHandle.value, entriesResource.value, handleSelectPostEditing])

  const handleCreateBlogPostClick = useCallback(() => {
    void handleCreateBlogPost()
  }, [handleCreateBlogPost])

  return (
    <ViewerStatusShell
      resource={state}
      state={state}
      loadingText="Loading blog..."
      emptyText="No sources configured for this blog"
      sources={sources}
    >
    <div className="bg-background-primary flex h-full w-full flex-col overflow-hidden">
      {/* Mode toggle header */}
      <div className="border-border flex h-9 shrink-0 items-center justify-between border-b px-3">
        <span className="text-foreground text-xs font-medium">
          {state.value?.name ?? 'Blog'}
        </span>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            onClick={handleSetReading}
            className={cn(
              'flex items-center gap-1 rounded px-2 py-0.5 text-xs',
              'hover:bg-list-hover-background',
              mode === 'reading' ? 'text-brand' : 'text-foreground-alt',
            )}
            title="Reading mode"
          >
            <LuBookOpen className="h-3 w-3" />
            Read
          </button>
          <button
            type="button"
            onClick={handleSetEditing}
            className={cn(
              'flex items-center gap-1 rounded px-2 py-0.5 text-xs',
              'hover:bg-list-hover-background',
              mode === 'editing' ? 'text-brand' : 'text-foreground-alt',
            )}
            title="Editing mode"
          >
            <LuPenLine className="h-3 w-3" />
            Edit
          </button>
        </div>
      </div>

      {/* Content area */}
      <div className="min-h-0 flex-1">
        {mode === 'reading' ?
          <BlogReadingView
            posts={blogPosts}
            selectedPost={selectedPost}
            onSelectPost={handleSelectPostReading}
            authorRegistry={authorRegistry}
          />
        : <div className="flex h-full overflow-hidden">
            {/* Post list sidebar */}
            <div
              className="border-r border-border"
              style={{ width: 250, minWidth: 250 }}
            >
              <NoteList
                source={firstSource}
                worldState={worldState}
                selectedNote={selectedPostName}
                onSelectNote={handleSelectPostEditing}
                onCreateNote={handleCreateBlogPostClick}
                renderEntryExtra={renderDraftBadge}
              />
            </div>

            {/* Editor area */}
            <div className="min-w-0 flex-1">
              {firstSource?.ref && selectedPostName ?
                <NoteContentView
                  worldState={worldState}
                  sourceRef={firstSource.ref}
                  noteName={selectedPostName}
                  editing={editing}
                  onToggleEdit={handleToggleEdit}
                />
              : <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
                  Select a post to edit
                </div>
              }
            </div>
          </div>
        }
      </div>
    </div>
    </ViewerStatusShell>
  )
}

export { BlogViewer, BlogTypeID }
export default BlogViewer
