import { useCallback, useMemo, useState } from 'react'

import { LuBookOpen, LuNewspaper, LuPenLine } from 'react-icons/lu'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import {
  ViewerStatus,
  resourceStatus,
  type ViewerStatusProps,
} from '@s4wave/web/object/ViewerStatus.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
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

// BlogViewer is the viewer for Notes Blog objects.
// Supports reading mode (blog reading view) and editing mode (NoteList + NoteContentView).
function BlogViewer({ objectInfo, worldState }: ObjectViewerComponentProps) {
  // The Blog object and its namespace for this viewer's persisted state.
  const objectKey = getObjectKey(objectInfo)
  const ns = useStateNamespace(['blog'])

  // Watch the Blog object and name the tab after it.
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

  // The source's UnixFS object and the folder inside it.
  const sourceObjectKey = parsed?.objectKey ?? ''
  const sourceSubpath = parsed?.path ?? ''

  // Access UnixFS root for reading blog posts.
  const rootHandle = useUnixFSRootHandle(worldState, sourceObjectKey)
  const pathHandle = useUnixFSHandle(rootHandle, sourceSubpath)
  const entriesResource = useUnixFSHandleEntries(pathHandle, {
    enabled: !!sourceObjectKey,
  })

  // Bumped on each save so the post list rereads.
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

  // Picking a post in the reading view opens it.
  const handleSelectPostReading = useCallback(
    (post: BlogPostData | null) => {
      setSelectedPostName(post?.name ?? '')
    },
    [setSelectedPostName],
  )

  // Picking a post in the editing list opens it for reading first.
  const handleSelectPostEditing = useCallback(
    (path: string) => {
      setSelectedPostName(path)
      setEditing(false)
    },
    [setSelectedPostName, setEditing],
  )

  // The edit press toggles the selected post's editor.
  const handleToggleEdit = useCallback(() => {
    setEditing((prev) => !prev)
  }, [setEditing])

  // The mode press switches to reading.
  const handleSetReading = useCallback(() => {
    setMode('reading')
  }, [setMode])

  // The mode press switches to editing.
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
        <span className="bg-brand/10 text-brand shrink-0 rounded px-1.5 py-0.5 text-xs leading-none font-medium">
          Draft
        </span>
      )
    },
    [draftNames],
  )

  // Create a new blog post with frontmatter template.
  const handleCreateBlogPost = useCallback(async () => {
    // A post needs the source folder open.
    const handle = pathHandle.value
    if (!handle) return

    // Pick the first new-post name no Markdown file holds.
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

    // A draft dated today, so it stays off the reading view until published.
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

    // Write the post and open it.
    await createTextFile(handle, name, template)
    handleSelectPostEditing(name)
  }, [pathHandle.value, entriesResource.value, handleSelectPostEditing])

  // The new post press creates a post from the template.
  const handleCreateBlogPostClick = useCallback(() => {
    void handleCreateBlogPost()
  }, [handleCreateBlogPost])

  // A save rereads the posts.
  const handleContentSaved = useCallback(() => {
    setContentVersion((prev) => prev + 1)
  }, [])

  // Show the read's status, or the empty blog, under the bar before its
  // posts. The mode press names the other mode, since switching to the
  // current one does nothing.
  const status: ViewerStatusProps | null =
    resourceStatus('the blog', state) ??
    (sources.length === 0
      ? { state: 'empty', message: 'This blog has no sources yet.' }
      : null)
  const modeAction =
    mode === 'reading'
      ? {
          id: 'mode',
          label: 'Edit posts',
          icon: <LuPenLine />,
          onClick: handleSetEditing,
        }
      : {
          id: 'mode',
          label: 'Read posts',
          icon: <LuBookOpen />,
          onClick: handleSetReading,
        }

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-hidden">
      <ViewerBar
        icon={<LuNewspaper />}
        title={state.value?.name || 'Blog'}
        actions={status ? [] : [modeAction]}
      />

      <div className="min-h-0 flex-1">
        {status ? (
          <ViewerStatus {...status} />
        ) : mode === 'reading' ? (
          <BlogReadingView
            posts={blogPosts}
            selectedPost={selectedPost}
            onSelectPost={handleSelectPostReading}
            authorRegistry={authorRegistry}
          />
        ) : (
          <div className="flex h-full overflow-hidden">
            {/* Post list sidebar */}
            <div className="border-border w-62.5 min-w-62.5 border-r">
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
            <div className="min-w-0 flex-1">
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
                <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
                  Select a post to edit
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export { BlogViewer, BlogTypeID }
export default BlogViewer
