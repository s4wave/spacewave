import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { Blog } from './proto/blog.pb.js'
import { Notebook } from './proto/notebook.pb.js'
import { applySeed, seedTypedObject, seedUnixfsRoot } from './seed-steps.js'

export function buildSeedBlogPost(blogName: string, date: string): string {
  return (
    '---\n' +
    'title: Hello World\n' +
    'date: ' +
    date +
    '\n' +
    'author: spacewave\n' +
    'summary: Welcome to ' +
    blogName +
    '.\n' +
    'tags: [welcome]\n' +
    'draft: false\n' +
    '---\n' +
    '\n' +
    '# Hello World\n' +
    '\n' +
    'This is your first post in ' +
    blogName +
    '.\n' +
    '\n' +
    'Edit this post from the companion notebook or switch this blog into edit mode to keep writing.\n'
  )
}

export function buildBlogUnixfsObjectKey(blogObjectKey: string): string {
  if (blogObjectKey.startsWith('blog/')) {
    return blogObjectKey.replace(/^blog\//, 'fs/blog/')
  }
  return blogObjectKey + '-fs'
}

// seedBlog writes a blog, the UnixFS root of its first post, and the
// companion notebook that edits its posts, as applySeed decides.
export async function seedBlog(
  worldState: IWorldState,
  blogObjectKey: string,
  blogName: string,
  description: string,
  authorRegistryPath: string,
  timestamp: Date,
  abortSignal?: AbortSignal,
): Promise<void> {
  const unixfsKey = buildBlogUnixfsObjectKey(blogObjectKey)
  await applySeed(
    worldState,
    seedTypedObject(
      'create blog object',
      blogObjectKey,
      Blog.toBinary({
        name: blogName,
        description,
        sources: [{ name: 'Posts', ref: unixfsKey + '/-/' }],
        authorRegistryPath,
      }),
      'notes/blog',
    ),
    [
      seedUnixfsRoot(
        'seed blog unixfs root',
        unixfsKey,
        [
          {
            path: 'hello-world.md',
            content: buildSeedBlogPost(
              blogName,
              timestamp.toISOString().slice(0, 10),
            ),
          },
        ],
        timestamp,
      ),
      // A blog op stored before applySeed decided seeds replays through
      // this handler, since stored op logs replay through the current
      // handler of an unpinned op id. The earlier handler kept any object
      // already at the companion notebook key, whatever its type, and wrote
      // the blog and its UnixFS root, so this seed keeps it too and the
      // replay reaches the same World.
      {
        ...seedTypedObject(
          'create companion notebook object',
          blogObjectKey + '-notebook',
          Notebook.toBinary({
            name: blogName + ' Notes',
            sources: [{ name: 'Posts', ref: unixfsKey + '/-/' }],
          }),
          'notes/notebook',
        ),
        keepFound: true,
      },
    ],
    abortSignal,
  )
}
