---
title: Command Line Basics
section: cli
order: 1
summary: Work with your Spaces and files from a terminal with the spacewave command.
---

The `spacewave` command lets you do much of what the app does, from a terminal.
You can sign in, list your Spaces, and read and write files. A Space is a
container for one project in Spacewave.

The command talks to Spacewave running on the same computer. If Spacewave is
not running, the command starts it.

## Check where you are

```sh
spacewave status
spacewave whoami
spacewave session list
spacewave session info
```

`status` shows whether Spacewave is running. `whoami` shows the account you are
signed in to. A session is one account signed in on this computer.
`session list` numbers each session. Commands use session 1 unless you pass
`--session-index`.

## Sign in

```sh
spacewave login
spacewave login --pem-file ./backup.pem
spacewave login local
spacewave logout
```

`login` signs in to a Spacewave Cloud account, or creates one. Add `--pem-file`
to sign in with a backup key instead of a password. `login local` creates an
account that stays on this computer.

## Look at your Spaces

```sh
spacewave space list
spacewave space create "My Space"
spacewave space info --space <space-id-or-name>
spacewave space settings --space <space-id-or-name>
```

`--space` picks a Space by name or ID. If you have only one Space open, you can
leave it out.

## Work with files

The `fs` commands list, read, create, move, and inspect files in a Space.

```sh
spacewave fs ls my-object
spacewave fs cat my-object/-/notes.txt
spacewave fs mkdir my-object/-/docs
spacewave fs write --from ./report.pdf my-object/-/docs/report.pdf
spacewave fs mv my-object/-/old.txt my-object/-/new.txt
spacewave fs stat my-object/-/docs/report.pdf
```

In these paths, `my-object` is an item in the Space, such as a Drive, and the
part after `/-/` is a path inside it. Short paths like these use your current
session and Space. You can also paste a full path from the browser address bar,
such as `/u/1/so/my-space/-/my-object/-/docs/report.pdf`.

## Open Spacewave in a browser

```sh
spacewave web --bg
spacewave web list
spacewave web stop <listener-id>
```

`spacewave web` serves Spacewave on a local address and prints a link you can
open in a browser on this computer. Without `--bg`, the address stays open
until you stop the command. With `--bg`, it keeps running after the command
returns. `web list` shows the running addresses, and `web stop` closes one.

## Stop Spacewave

```sh
spacewave stop
```

`stop` shuts down Spacewave running in the background on this computer. If it
is not running, the command says so.

## Connect to a specific copy

The command finds Spacewave on its own. To connect to one specific copy, set
`--socket-path` or `SPACEWAVE_SOCKET_PATH` to its socket file. The command then
connects only to that socket and does not start Spacewave.
