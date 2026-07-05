---
title: Command Line Basics
section: cli
order: 1
summary: Use the native CLI to inspect sessions, Spaces, files, and local runtime state.
---

The `spacewave` CLI talks to a Spacewave desktop app or daemon. Without an
explicit socket path, most commands resolve the state path and can autostart the
daemon. With `--socket-path` or `SPACEWAVE_SOCKET_PATH`, the CLI connects only
to that socket.

## Start with session commands

```sh
spacewave status
spacewave whoami
spacewave session list
spacewave session info
```

`status` checks daemon health. `whoami` prints the current identity. `session
list` shows configured sessions with index, session ID, provider, and account.
Most commands default to session index 1 unless you pass `--session-index`.

## Sign in

```sh
spacewave login
spacewave login --pem-file ./backup.pem
spacewave login local
spacewave logout
```

Password login signs into or creates a Spacewave Cloud account. PEM login uses a
backup key. `login local` creates a local offline account. `logout` signs out the
selected local session target.

## Inspect Spaces

```sh
spacewave space list
spacewave space create "My Space"
spacewave space info --space <space-id-or-name>
spacewave space settings --space <space-id-or-name>
```

`space list` can watch for changes. `space info` and `space settings` inspect
the mounted Space selected by `--space` or by the daemon's default Space when
only one Space is available.

## Work with files

The `fs` command works on UnixFS objects inside a Space.

```sh
spacewave fs ls my-object
spacewave fs cat my-object/-/notes.txt
spacewave fs mkdir my-object/-/docs
spacewave fs write --from ./report.pdf my-object/-/docs/report.pdf
spacewave fs mv my-object/-/old.txt my-object/-/new.txt
spacewave fs stat my-object/-/docs/report.pdf
```

Short file URIs use the default session and Space. Full URIs match web paths,
such as `/u/1/so/my-space/-/my-object/-/docs/report.pdf`.

## Open local web access

```sh
spacewave web --bg
spacewave web list
spacewave web stop <listener-id>
```

`spacewave web` starts a localhost web listener for the native runtime. A
background listener stays attached to the daemon. Foreground listeners stay open
until you stop the process.
