// Author represents the display data for a blog post author.
export interface Author {
  name: string
  avatar: string
  url: string
  bio: string
}

// authors maps author slugs to their display data.
export const authors: Record<string, Author> = {
  paralin: {
    name: 'Christian Stewart',
    avatar: 'https://github.com/paralin.png',
    url: 'https://github.com/paralin',
    bio: 'Founder, Aperture Robotics',
  },
}
