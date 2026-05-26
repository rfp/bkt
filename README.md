# 🌈 bkt

> A tiny `glab`-style CLI for Bitbucket Cloud, born from a vibecoding session and currently held together by Go, optimism, and HTTP requests.

<p>
  <img alt="status" src="https://img.shields.io/badge/status-vibecoded-ff69b4">
  <img alt="language" src="https://img.shields.io/badge/language-Go-00ADD8">
  <img alt="bitbucket" src="https://img.shields.io/badge/target-Bitbucket%20Cloud-0052CC">
  <img alt="mvp" src="https://img.shields.io/badge/stage-MVP-orange">
  <img alt="license" src="https://img.shields.io/badge/license-MIT-green">
</p>

`bkt` is a small command-line tool inspired by `glab`, but aimed at **Bitbucket Cloud**.

The idea is simple:

```bash
bkt pr list
bkt pr view 123
bkt pr create
bkt pipeline list
```

Less browser hopping. More terminal flow.

## ✨ Vibe check

This project was created through **vibecoding**: a fast, conversational, AI-assisted coding session where the first goal was to get something real into a repo instead of polishing architecture diagrams until the moon gets bored.

That means:

- it is intentionally small;
- it is an MVP;
- some things are rough;
- the command shape matters more than perfection right now;
- future refactors are expected, welcome, and probably inevitable.

If you are looking for a pristine enterprise SDK, this is not that. Yet.

If you are looking for a useful little Bitbucket CLI seedling, welcome. 🌱

## 📡 Project status

`bkt` is currently in **early MVP / traction-check mode**.

The core is usable, tested and releasable, but the next features will depend on real feedback rather than a giant speculative roadmap.

Right now, the most useful feedback is:

- does it work with your real Bitbucket Cloud repositories?
- which command do you miss first?
- did authentication or setup feel awkward?
- did Homebrew or the install script work for you?
- did any command feel unsafe, surprising or too noisy?

If you try it and something feels off, please open an issue. Small, concrete reports are more useful than grand feature essays.

## 🚀 Quick start

Install with Homebrew:

```bash
brew tap rfp/bkt
brew install rfp/bkt/bkt
```

Use the fully-qualified formula name `rfp/bkt/bkt`: another Homebrew package is also named `bkt`, so plain `brew install bkt` may install the wrong project.

Or install the latest release on Linux/macOS with the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/rfp/bkt/main/install.sh | BKT_VERSION=v0.1.0 sh
```

You can choose the install directory:

```bash
curl -fsSL https://raw.githubusercontent.com/rfp/bkt/main/install.sh | BKT_VERSION=v0.1.0 BKT_INSTALL_DIR="$HOME/.local/bin" sh
```

Or clone and build:

```bash
git clone https://github.com/rfp/bkt.git
cd bkt
go build -o bkt .
./bkt help
```

Run the test suite:

```bash
go test ./...
```

Optionally move a local build to your PATH:

```bash
sudo mv bkt /usr/local/bin/bkt
```

Then:

```bash
bkt auth login
```

## 🔐 Authentication

Create a **Bitbucket Cloud API token** with scopes for repositories, pull requests, and pipelines.

Then run:

```bash
bkt auth login
```

The token prompt uses hidden terminal input, so the API token is not echoed while typing.

`bkt` stores non-sensitive config here:

```text
~/.config/bkt/config
```

That file contains values such as email, username, workspace, and API base URL. It should **not** contain `token=`.

The API token is stored in the operating system keychain:

- macOS: Keychain
- Linux: Secret Service / libsecret
- Windows: Credential Manager

If the token cannot be stored in the keychain, login fails. There is intentionally no plain-text token fallback.

## 🌐 API host support

For now, `bkt` supports **Bitbucket Cloud only**.

The only accepted API base URL is:

```text
https://api.bitbucket.org/2.0
```

This is intentional: the CLI refuses other hosts and non-HTTPS URLs before sending credentials anywhere.

Bitbucket Data Center support may be added later, but it will require explicit configuration and separate safety checks.

## 🧭 Commands

### Version

```bash
bkt version
bkt --version
bkt -v
```

Development builds report:

```text
bkt version dev
commit unknown
built unknown
```

Release builds can inject version metadata with Go linker flags.

### Auth

```bash
bkt auth login
bkt auth status
bkt auth logout
```

### Repository

```bash
bkt repo view
bkt repo view --json
```

### Pull requests

```bash
bkt pr list
bkt pr list --state MERGED
bkt pr view 123
bkt pr view 123 --web
bkt pr create --title "Fix login" --description "Adds validation" --target main
bkt pr checkout 123
bkt pr approve 123
bkt pr merge 123
```

`bkt pr checkout <id>` fetches the PR source branch, then checks it out locally as:

```text
pr/<id>
```

For example:

```bash
bkt pr checkout 123
# local branch: pr/123
```

This avoids accidentally reusing or overwriting a local branch with the same name as the remote PR source branch.

### Pipelines

```bash
bkt pipeline list
bkt pipeline list --json
bkt pipeline run --branch main
```

## 🧯 Write operation confirmations

Commands that change remote state ask for confirmation by default:

```bash
bkt pr approve 123
bkt pr merge 123
bkt pipeline run --branch main
```

For scripts and automation, pass `--yes` explicitly:

```bash
bkt pr approve 123 --yes
bkt pr merge 123 --yes
bkt pipeline run --branch main --yes
```

The default answer is No.

## 🧠 How repo detection works

Most commands expect to be run inside a local Git repository whose `origin` remote points to Bitbucket Cloud.

Supported remote formats:

```text
git@bitbucket.org:workspace/repository.git
https://bitbucket.org/workspace/repository.git
```

From that, `bkt` extracts:

```text
workspace = workspace
repo      = repository
```

Then it talks to the Bitbucket Cloud REST API.

## 🤖 JSON output

Several commands support `--json`, useful for scripts and automations:

```bash
bkt pr list --json
bkt repo view --json
bkt pipeline list --json
```

Example vibe:

```bash
bkt pr list --json | jq '.[] | {id, title, state}'
```

## 📦 Releases

Releases are built with GoReleaser.

To create a release, tag `main` with a version tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release workflow runs tests first, then builds archives for macOS, Linux and Windows, with checksums attached to the GitHub Release.

## 🛠 Current shape

Right now, this is deliberately a **single-file Go CLI**.

That is not the final architecture. It is the first working cut.

Expected future structure:

```text
cmd/
internal/bitbucket/
internal/config/
internal/git/
internal/output/
```

But before splitting files, the project should earn the complexity.

## 🗺 Roadmap

The roadmap is intentionally small until there are real users asking for real things.

Done:

- [x] Secure credential storage via OS keychain.
- [x] Add tests.
- [x] Add GitHub Actions.
- [x] Add release builds.
- [x] Add Homebrew tap.
- [x] Add Linux/macOS install script.

Possible next steps, depending on traction:

- [ ] Split code into packages.
- [ ] Add deb/rpm packages.
- [ ] Add Scoop manifest.
- [ ] Add `bkt repo clone`.
- [ ] Add `bkt pr comment`.
- [ ] Add `bkt pr diff`.
- [ ] Add `bkt pipeline view`.
- [ ] Add `bkt pipeline logs`.
- [ ] Investigate Bitbucket Data Center support.

## 🧪 MVP warning label

This project can talk to real Bitbucket repositories.

Read commands are safer. Write commands such as these affect real remote state:

```bash
bkt pr approve 123
bkt pr merge 123
bkt pipeline run
```

Use with attention, especially while the tool is still young and caffeinated.

## 💡 Why?

GitHub has `gh`.

GitLab has `glab`.

Bitbucket Cloud deserves a pleasant terminal companion too.

This is the first spark.

## 📜 License

MIT License.

Small, permissive, and friendly to forks, experiments, internal company use, and random weekend terminal sorcery.
