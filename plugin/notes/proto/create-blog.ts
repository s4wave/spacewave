// CREATE_BLOG_OP_ID is the operation type ID for the blog create op that builds
// its content under the World writer. Only stored ops carry it: they replay
// through the current handler of an unpinned op id, so its handler stays and
// must keep their result. New creates send CREATE_BLOG_STAGED_OP_ID.
export const CREATE_BLOG_OP_ID = 'notes/blog/create'

// CREATE_BLOG_STAGED_OP_ID is the operation type ID for the blog create op
// whose sender builds the content in a World stage; see CreateBlogStagedOp.
export const CREATE_BLOG_STAGED_OP_ID = 'notes/blog/create-staged'

// BLOG_OBJECT_KEY is the default object key for the blog quickstart.
export const BLOG_OBJECT_KEY = 'blog/site'
