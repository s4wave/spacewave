# Contributing

Contributions are welcome via pull requests. All code is released under the
project's license (see LICENSE).

## Developer Certificate of Origin

All commits must include a
[DCO](https://developercertificate.org/) sign-off line certifying you have the
right to submit the code under this project's license.

Add it automatically with `-s`:

```sh
git commit -s -m "fix: correct edge case in example"
```

To sign off commits you have already made:

```sh
git commit --amend --signoff --no-edit  # last commit
git rebase --signoff HEAD~N             # last N commits
```

Sign-off is verified on every pull request. PRs with unsigned commits will not
be merged.

## Pull Requests

- Keep changes focused. One logical change per PR.
- Follow the existing code style: `gofmt`, Oxlint, Oxfmt.
- Ensure CI passes (`bun run ci`).
