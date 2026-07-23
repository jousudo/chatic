# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-07-23

Security hardening pass against the OWASP Top 10 for LLM Applications (2025),
plus two correctness fixes. No breaking changes.

### Security

- **Unbounded Consumption (LLM10):** reject oversized direct-message text before it
  enters the processing queue; add a per-user rolling rate limit (12 messages / 30s)
  on DM processing; give the direct Google/Translate TTS HTTP calls a 20s timeout so a
  hung provider can no longer stall the single FIFO worker.
- **Prompt Injection (LLM01):** strengthen the shared anti-injection clause to treat
  student input as delimited data and explicitly refuse persona swaps
  ("you are now…", "act as…").

### Fixed

- XP link bonus is now awarded for URLs embedded mid-sentence (e.g. "Read this:
  https://…"), not only when the message starts with the link.
- The admin panel HTTP server now fails fast (aborts) if it cannot bind its port,
  instead of silently running headless without the only pairing / key-management path.

### Changed

- `TutorEngine.BuildSystemInstruction` no longer reads the global config; the custom
  system-prompt override is injected by the caller (decoupling for testability and to
  avoid a config-reload race).

### Added

- Community & project health files: `CONTRIBUTING.md`, `SECURITY.md`,
  `CODE_OF_CONDUCT.md`, issue/PR templates, and a CI workflow.

## [0.1.0] - 2026-07-08

First public release.

### Added

- Private, self-hosted multilingual language tutor for WhatsApp (text + optional
  voice/TTS via FFmpeg).
- Multi-LLM support with failover (Gemini → OpenAI → Claude → Ollama) and a
  per-user/per-group/system key hierarchy; Gemini free-key round-robin pool.
- Onboarding flow with native/target language, level, interests, and a
  student-chosen teacher name.
- Study commands: `/grammar`, `/word`, `/vocab`, `/quiz`, `/fix`, plus incidental
  correction (`💡 Quick Tip`), `/tips`, `/ranking`.
- Study groups: reactive replies (`/ask`, `/correct`, @mention) and Phase 2
  activities (`/gquiz` native polls, `/greveal`, `/gword`, `/gchallenge`,
  `/ghelp`) with per-group rate limiting.
- News-link ingestion (SSRF-guarded) and PDF document ingestion (pure-Go).
- Optional multi-account mode: household members pair their own WhatsApp and use
  the tutor via self-chat.
- Web admin panel: pairing (QR / phone code), whitelist, users, and encrypted
  API-key entry.
- Privacy / LGPD: chat content and API keys encrypted at rest (AES-GCM), zero
  logging of message content, right-to-erasure (`/forget`, admin delete).
- Packaging: cross-platform static binaries, `.deb`/`.rpm` packages with systemd
  hardening + AppArmor, Docker image, and one-line installers with best-effort
  FFmpeg setup.

[Unreleased]: https://github.com/jousudo/chatic/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/jousudo/chatic/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/jousudo/chatic/releases/tag/v0.1.0
