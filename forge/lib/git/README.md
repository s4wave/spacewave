# Git

> Library of Git targets for Forge.

## Introduction

These controllers implement the Git operations against world objects:

- clone (creates Repo object)
- checkout (creates/updates Worktree object + Unixfs working directory object)
- stage (i.e. git add) (updates Worktree from Working directory object)
- commit (updates Repo object from Worktree object)

This is done with the following World object types:

 - Repo: git repository
 - Worktree: information about a workdir
 - Workdir (Unixfs): working directory with unixfs

The workdir can be mounted with a link to another Unixfs tree to implement
"checking out" the git repository somewhere.

The fuse mount could be configured to use the Worktree (and Repo) to also
implement a virtual ".git" tree so that the standard Git client operations can
be mapped into the block-graph backed Git repository.
