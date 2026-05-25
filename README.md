# bkt

A tiny `glab`-style CLI for Bitbucket Cloud.

This is an MVP, intentionally small and dependency-free. It uses the Bitbucket Cloud REST API v2.

## Install locally

```bash
go build -o bkt .
./bkt help
```

Optionally move it to your PATH:

```bash
sudo mv bkt /usr/local/bin/bkt
```

## Authentication

Create a Bitbucket Cloud API token with scopes for repository, pull requests, and pipelines.

Then run:

```bash
bkt auth login
```

This MVP stores the token in `~/.config/bkt/config` with `0600` permissions. The next hardening step is replacing this with macOS Keychain / Linux Secret Service / Windows Credential Manager.

## Commands

```bash
bkt auth login
bkt auth status
bkt auth logout

bkt repo view

bkt pr list
bkt pr view 123
bkt pr create --title "Fix login" --description "Adds validation" --target main
bkt pr checkout 123
bkt pr approve 123
bkt pr merge 123

bkt pipeline list
bkt pipeline run --branch main
```

Most commands expect to be run inside a local Git repository whose `origin` remote points to Bitbucket Cloud, for example:

```text
git@bitbucket.org:workspace/repository.git
https://bitbucket.org/workspace/repository.git
```

## JSON output

Several commands support `--json`:

```bash
bkt pr list --json
bkt repo view --json
bkt pipeline list --json
```

## Roadmap

- Secure credential storage via OS keychain.
- `bkt repo clone`.
- `bkt pr comment`.
- `bkt pr diff`.
- `bkt pipeline view`.
- `bkt pipeline logs`.
- Homebrew tap and release builds.
- Bitbucket Data Center support.
