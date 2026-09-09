# prodtop

prodtop shows what your production database is doing, without leaving the
terminal. Point it at Postgres, get live activity, blocking locks, table
growth and a SQL runner with CSV export. It starts read-only and stays that
way unless you ask otherwise.

Built with Go and Bubble Tea. Single binary, no server, your queries never
leave your machine.

## Install

Requires Go 1.24 or newer.

    go install github.com/prodtop/prodtop/cmd/prodtop@latest

Or build it yourself:

    git clone https://github.com/prodtop/prodtop
    cd prodtop
    go build -o bin/prodtop ./cmd/prodtop

## Quick start

Copy the example config next to the binary and edit the DSN:

    cp config.example.yaml prodtop.yaml
    ./bin/prodtop

No config file? prodtop still opens, with an empty target list.

## Demo

The demo folder spins up a local Postgres and Redis with sample data:

    cd demo
    docker compose up -d

Then point prodtop at it from the repo root:

    ./bin/prodtop --config demo/prodtop.demo.yaml

## Config

prodtop looks for a config file in this order: `--config` flag,
`PRODTOP_CONFIG` env, `~/.config/prodtop/config.yaml`, `./prodtop.yaml`.

    refresh_seconds: 2
    query_timeout_seconds: 5
    postgres:
      - name: prod
        dsn: postgres://app:changeme@db.internal:5432/appdb?sslmode=require
    redis:
      - name: cache
        addr: cache.internal:6379
        password: ""
        db: 0

## Keys

    j / k         move
    tab           switch between services and main panel
    1 - 5         Activity / Locks / Tables / SQL / Redis
    enter         open service, refresh
    r             refresh
    K             terminate backend (activity tab, asks first)
    i             edit sql (sql tab), enter runs it, esc stops
    o / f         load next favorite / save favorite (sql tab)
    x             clear sql results
    e             export current view to csv
    ?             help
    q             quit

## Safety

prodtop connects read-only by default. Statements that change data
(INSERT, UPDATE, DELETE) are rejected unless you pass `--write`, and every
guarded action is appended to `~/.local/share/prodtop/audit.jsonl`.
Passwords in the config are never printed back; DSNs are masked in output.

## Roadmap

Redis views, command palette, query filter, demo dataset. They are tracked
as milestones and land in order.
