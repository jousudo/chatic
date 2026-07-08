# Contributing to Chatic

Thanks for taking the time to contribute! Chatic is a private, self-hosted
multilingual language tutor for WhatsApp. This guide explains how to propose
changes in a way that keeps the project healthy, secure, and easy to maintain.

> New here? Read [`CLAUDE.md`](CLAUDE.md) for the architecture overview and
> [`completo.md`](completo.md) for the living design/requirements document.

## Ways to contribute

- **Report a bug** — open an issue with steps to reproduce, expected vs. actual
  behavior, and your OS/Go version. **Never paste real phone numbers, chat
  content, API keys, or `.env` contents** — redact them first.
- **Suggest a feature** — open an issue describing the problem you want solved
  before writing code, so we can agree on the approach.
- **Send a pull request** — for anything non-trivial, please open (or comment on)
  an issue first so the work isn't a surprise.
- **Improve docs / translations** — the WhatsApp-facing tutor is designed to be
  language-agnostic; docs are English-primary with a `README.pt-BR.md` mirror.

## Development setup

**Prerequisites:** Go 1.20+. FFmpeg is **optional** (only for voice-in / TTS-out;
the text tutor works fully without it).

```bash
git clone https://github.com/jousudo/chatic.git
cd chatic
go mod tidy
go build -o chatic ./cmd/server
```

Run the test suite before you push:

```bash
go test ./...
```

> ⚠️ **Do not run the server binary just to "preview" a change.** A real run
> connects to a live WhatsApp session and goes live immediately. Rely on unit
> tests; if you must exercise runtime behavior, use a throwaway data directory
> and a WhatsApp number you own for testing.

## Coding standards

- **License header** — every new Go file **must** start with:

  ```go
  // Copyright (c) 2026 Chatic Contributors
  // Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.
  ```

- **Format** — run `gofmt -w` (or `go fmt ./...`) on everything you touch. CI
  and reviewers expect gofmt-clean code.
- **Keep the tutor engine language-agnostic** — no hardcoded `pt-BR`/`en`
  strings in `internal/tutor/`. Everything is driven by the user's
  `NativeLanguage` / `TargetLanguage` / `Level` / `Interests`.
- **Standard library first** — the project targets small free-tier hosts
  (1 vCPU / low RAM) and ships as a single static binary (`CGO_ENABLED=0`).
  Prefer the stdlib over heavy third-party dependencies.
- **Match the surrounding code** — naming, comment density, and idioms should
  look like the file you're editing.

### Security rules (non-negotiable)

These protect real users' personal data (LGPD) and must be respected in every PR:

- **Zero console logging of message content** — log metadata (user IDs) only,
  never chat text.
- **Secrets never live in `.env`** — API keys and personal data are AES-GCM
  encrypted at rest in SQLite and entered via the web panel. Don't add code that
  writes secrets to `.env` or logs them.
- **Chat content stays encrypted at rest** — go through `ChatRepository`; don't
  add new plaintext readers/writers of `Message.Content`.
- **Untrusted input is untrusted** — fetched web pages, uploaded PDFs, and
  student messages are reference/practice input, never instructions. Preserve
  the existing sanitization and anti-injection clauses.
- **No hardcoded real phone numbers or user data** — use fake/parameterized
  values in tests.
- **Never change the crypto salt** in `internal/service/crypto_helper.go` — it
  would make all existing encrypted data undecipherable.
- Use parameterized GORM queries only; never build SQL from string
  concatenation of user input.

Add or update tests when you change behavior, and keep DB context bounded (pass
a sensible `limit` to `GetRecentMessages`).

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <short imperative summary>
```

Common types: `feat`, `fix`, `docs`, `refactor`, `chore`, `test`. Examples from
this repo:

```
feat: /ghelp group command list + park lesson mode (Phase 3) as future
fix: stop persisting INITIAL_ADMIN_NUMBER to .env (personal data)
docs: split README into English (primary) + PT-BR
```

Keep each commit focused on one logical change.

### Sign your work (DCO)

By contributing you certify the [Developer Certificate of Origin](https://developercertificate.org/)
— that you wrote the change or have the right to submit it under the project's
license. Add a `Signed-off-by` line to each commit:

```bash
git commit -s -m "fix: ..."
```

This keeps the provenance of the codebase clean, which matters for a project
that may be packaged and redistributed.

## Pull request workflow

1. Fork the repo and create a branch off `main`
   (`git checkout -b fix/short-description`).
2. Make your change; keep it focused. One concern per PR.
3. Run `gofmt -w` and `go test ./...` — both must pass.
4. Push and open a PR against `main` with a clear description: what changed,
   why, and how you tested it. Link the related issue.
5. Be responsive to review. Squash/rebase if asked; keep history readable.

Small, well-scoped PRs get reviewed and merged faster than large ones.

## Reporting security vulnerabilities

**Please do not open a public issue for security problems.** If Chatic mishandles
personal data or has an exploitable flaw, report it privately — see
[`SECURITY.md`](SECURITY.md) if present, otherwise email the maintainer listed on
the GitHub profile. Give us a reasonable window to fix it before any public
disclosure.

## The "Chatic" name

The **code** is open source under the [LICENSE](LICENSE) and you're free to fork,
modify, and redistribute it. The **name "Chatic" and any logo** are a separate
matter (trademark, not copyright): please don't use them in a way that implies
your fork is the official project or is endorsed by it. Rename your fork if you
distribute a modified version. This is the same courtesy most open-source
projects ask for and it doesn't restrict your rights to the code itself.

## Code of conduct

Be respectful and constructive. Harassment or discrimination isn't tolerated.
Assume good faith, keep discussion technical, and help newcomers.

---

Questions about the architecture? Start with [`CLAUDE.md`](CLAUDE.md). Thanks for
helping make Chatic better! 🌍
