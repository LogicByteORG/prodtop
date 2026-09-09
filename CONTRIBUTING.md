# Contributing to prodtop

Thanks for stopping by. This is a small project with one rule that matters:
keep the codebase easy to change. Everything below serves that.

## Getting started

You need Go 1.24 or newer. No Docker, no services, no setup script.

    go build -o bin/prodtop ./cmd/prodtop
    ./bin/prodtop

Without a config file the app opens with an empty target list, so you can
click through the UI right away. To see real data, copy
`config.example.yaml` to `prodtop.yaml` and point a DSN at any Postgres you
can read from. Copy `config.example.yaml`, never commit a real DSN.

Useful commands (see Makefile for the full list):

    make run     run the TUI
    make vet     static checks, must pass
    make test    unit tests, must pass
    gofmt -l .  must print nothing

## How to send a change

1. Fork the repo and create a branch with a plain name: `redis-keys`,
   `fix-activity-sort`, that kind of thing.
2. Keep the change focused. One pull request, one idea.
3. Run `make vet`, `make test` and `gofmt -l .` before pushing.
4. Open the pull request against `main` and say what it does and why.
   If it touches the TUI, attach a screenshot or a short GIF, terminal
   layouts are impossible to review blind.

There is no commit message police, but `area: what changed` reads well:
`postgres: sort activity by duration`, `tui: wrap long queries in detail`.

## House rules

These are not optional, they are what keeps the project maintainable:

- No explanatory comments in code. If a line needs a comment to be
  understood, rename something until it does not.
- `gofmt`, `go vet` and `go test ./...` stay green. A red main branch
  blocks everything else.
- One binary, no new processes or servers. Adapters talk to external
  services, the TUI never phones home.
- Read-only stays the default. Anything that can change production state
  goes behind the write guard and writes an audit entry, no exceptions.
- Dependencies are added reluctantly. The standard library first, a new
  module only when it clearly earns its place.

## Reporting bugs

Open an issue with three things: what you ran (command and flags), what you
expected, and what happened instead. Paste the notice-line error if there is
one. `prodtop --version` output helps.

## License

By contributing you agree that your work lands under the Apache-2.0 license,
same as the rest of the repo.
