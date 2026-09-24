---
title: Large Files in Git
section: cli
order: 2
summary: Keep a Git repository's large files in a Space with Git LFS.
---

Git LFS keeps large files, such as images, video, and audio, out of a Git
repository's history. The repository stores a small pointer for each file, and
the files themselves live somewhere else. Spacewave can be that somewhere else:
one command points a repository's large files at a Space, and your Git history
stays wherever it is today, such as GitHub.

You need [Git LFS](https://git-lfs.com) installed.

## Set up a repository

Run this inside the repository:

```sh
spacewave git lfs setup --space "My Space"
```

Setup creates a folder-like item in the Space named `git-lfs/<repository>`
(pass another name after `setup` to choose it), and tells Git LFS to send large
files there. Running it again changes nothing.

Then choose which files are large and push as usual:

```sh
git lfs track '*.png' '*.mp4'
git add .gitattributes
git commit -m "chore: track media with Git LFS"
git push
```

`git push` uploads the large files to the Space, then waits until Spacewave has
finished syncing them before it pushes your commits. Anyone who fetches your
commits can then download the files.

## Clone a repository that uses it

A fresh clone does not know about the Space yet, so it cannot download the
large files on its own. Clone without them, run setup with the same Space and
name, then pull them:

```sh
GIT_LFS_SKIP_SMUDGE=1 git clone https://github.com/example/media.git
cd media
spacewave git lfs setup --space "My Space" git-lfs/media
git lfs pull
```

## Move a repository that already uses Git LFS

When the repository already keeps large files on another Git LFS server, such
as GitHub, setup lists how many and prints the two commands that move them.
Download every version from the old server, bypassing the Space, then upload
them all to the Space:

```sh
git -c lfs.standalonetransferagent= lfs fetch --all origin
git lfs push --all origin
```

After the move, new large files go only to the Space. Anyone who fetches the
files without running setup still reads the old server, which stops receiving
them.

## Share with collaborators

The large files are only as available as the Space. A collaborator needs access
to the Space and the `spacewave` command to push or download them; someone who
can read the Git repository but not the Space sees only the pointers. To work
together, [share the Space](/docs/users/spaces/share-spaces-and-organizations)
with each collaborator.

You can see the stored files in the Space: they are kept under `objects/` in the
`git-lfs/<repository>` item, named by their content hash.
