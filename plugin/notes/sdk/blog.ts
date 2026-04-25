import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { BlogResourceServiceClient } from './blog_srpc.pb.js'
import type { Blog } from '../proto/blog.pb.js'
import type { WatchBlogResponse } from './blog.pb.js'

// BlogTypeID is the type identifier for blog objects.
export const BlogTypeID = 'spacewave-notes/blog'

// IBlogHandle contains the BlogHandle interface.
export interface IBlogHandle {
  // watchBlog streams the current blog state.
  watchBlog(abortSignal?: AbortSignal): AsyncIterable<Blog>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// BlogHandle represents a handle to a blog resource.
// Each instance maps 1:1 to a BlogResource on the backend.
export class BlogHandle extends Resource implements IBlogHandle {
  private service: BlogResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new BlogResourceServiceClient(resourceRef.client)
  }

  // watchBlog streams the current blog state.
  public async *watchBlog(
    abortSignal?: AbortSignal,
  ): AsyncIterable<Blog> {
    const stream = this.service.WatchBlog({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchBlogResponse>) {
      if (resp.blog) {
        yield resp.blog
      }
    }
  }
}
