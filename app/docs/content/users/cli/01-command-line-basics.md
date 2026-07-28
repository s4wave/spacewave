---
title: Command Line Basics
section: cli
order: 1
summary: Work with your Spaces and files from a terminal with the spacewave command.
---

The `spacewave` command does from a terminal much of what the app does in a
window: sign in, list your Spaces, and move files around. It talks to Spacewave
running on the same machine, and starts it if it is not already running.

## See where you stand

```sh
spacewave status
spacewave whoami
spacewave session list
spacewave session info
```

`status` tells you whether Spacewave is up. `whoami` prints who you are signed
in as. `session list` numbers each account you have set up on this machine.
Commands use number 1 unless you pass `--session-index`.

## Sign in

```sh
spacewave login
spacewave login --pem-file ./backup.pem
spacewave login local
spacewave logout
```

Plain `login` signs into a Spacewave Cloud account, or creates one. Pass
`--pem-file` to sign in with a backup key instead of a password. `login local`
sets up an account that stays on this machine.

## Look at your Spaces

```sh
spacewave space list
spacewave space create "My Space"
spacewave space info --space <space-id-or-name>
spacewave space settings --space <space-id-or-name>
```

Pass `--space` to pick one by name or id. If you only have one Space open, you
can leave it out.

## Work with files

The `fs` commands do what you would expect on the files in a Space.

```sh
spacewave fs ls my-object
spacewave fs cat my-object/-/notes.txt
spacewave fs mkdir my-object/-/docs
spacewave fs write --from ./report.pdf my-object/-/docs/report.pdf
spacewave fs mv my-object/-/old.txt my-object/-/new.txt
spacewave fs stat my-object/-/docs/report.pdf
```

Short paths like these use the account and Space you are already on. You can
also paste a full path copied from the browser address bar, such as
`/u/1/so/my-space/-/my-object/-/docs/report.pdf`.

## Reach it from a browser

```sh
spacewave web --bg
spacewave web list
spacewave web stop <listener-id>
```

`spacewave web` opens Spacewave on a local address you can visit in a browser.
With `--bg` it keeps running after the command returns; without it, it stays up
until you stop the command.

## Pointing it somewhere else

By default the command finds Spacewave on its own. Set `--socket-path` or
`SPACEWAVE_SOCKET_PATH` to connect to a specific one instead, and it will use
only that.
