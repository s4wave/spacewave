import christianStewartAvatar from '@s4wave/web/images/christian-stewart.png'

export interface Author {
  name: string
  avatar: string
  url: string
  bio: string
}

export const authors: Record<string, Author> = {
  paralin: {
    name: 'Christian Stewart',
    avatar: christianStewartAvatar,
    url: 'https://github.com/paralin',
    bio: 'Founder, Aperture Robotics',
  },
}
