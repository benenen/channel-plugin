# Root Main Data Dir Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Put the root cobra command back into `main.go`, keep subcommands in `cmd/`, add a default data root at `~/.myclaw`, default SQLite at `~/.myclaw/myclaw.db`, and give each bot a workspace under `~/.myclaw/bots/<bot_id>/workspace`.

**Architecture:** `main.go` owns root command parsing, default `server` execution, and top-level usage/error behavior. The `cmd` package owns concrete subcommands and server runtime helpers. The config package resolves all default filesystem paths from `CHANNEL_DATA_DIR`, and the bot CLI resolver maps each bot to a deterministic workspace path derived from that data root.

**Tech Stack:** Go 1.23, cobra, standard library `os`/`path/filepath`, existing config/bootstrap/bot packages, `go test`

---
