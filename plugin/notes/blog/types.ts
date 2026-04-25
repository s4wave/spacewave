// BlogPostData represents a parsed blog post from a markdown file with frontmatter.
export interface BlogPostData {
  // name is the original .md filename.
  name: string
  // title is the post title from frontmatter or derived from filename.
  title: string
  // date is the publication date string (YYYY-MM-DD) from frontmatter.
  date: string
  // summary is the optional post summary from frontmatter.
  summary: string
  // tags is the list of tags from frontmatter.
  tags: string[]
  // body is the markdown content after frontmatter.
  body: string
  // author is the optional author name from frontmatter.
  author?: string
  // draft indicates the post is a draft and should be hidden from reading view.
  draft?: boolean
}
