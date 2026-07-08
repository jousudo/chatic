# Security Policy

Chatic handles **personal data** (WhatsApp numbers, chat content, API keys) and
is designed around privacy (LGPD). We take security reports seriously.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security problems.** Public
disclosure before a fix puts real users' data at risk.

Instead, report privately using one of:

- **GitHub private advisory** (preferred): open a report at
  <https://github.com/jousudo/chatic/security/advisories/new>
- **Email:** contact the maintainer via the email on the
  [GitHub profile](https://github.com/jousudo).

Please include:

- A description of the issue and its impact.
- Steps to reproduce (a minimal proof-of-concept if possible).
- Affected version/commit and your environment (OS, Go version).
- **Redact any real personal data** — never send real phone numbers, chat
  content, `.env` files, or live API keys. Use placeholders.

## What to expect

- We aim to acknowledge a valid report within **7 days**.
- We'll work with you on a fix and coordinate a disclosure timeline. Please give
  us a **reasonable window** (typically up to 90 days) before disclosing
  publicly.
- With your permission, we'll credit you in the release notes.

## Scope

This is a self-hosted application: **you run your own instance and control your
own data.** Reports are most valuable when they concern the code itself, e.g.:

- Ways to bypass the whitelist / personal-device self-chat scoping.
- Leakage of secrets (API keys, master key) or chat content into logs, `.env`,
  or unencrypted storage.
- Prompt-injection paths from untrusted input (fetched links, uploaded PDFs,
  student messages) that reach privileged behavior.
- SSRF, SQL injection, session/auth flaws in the web admin panel.
- Weaknesses in the at-rest encryption (`internal/service/crypto_helper.go`,
  `keyvault.go`).

**Out of scope:** issues that require an attacker to already control the host or
your `.env`/`storage/` directory; misconfiguration of your own deployment;
vulnerabilities in third-party dependencies (report those upstream, though a
heads-up is welcome).

## Supported versions

Until a stable `1.0`, only the latest release / `main` branch receives security
fixes.

Thank you for helping keep Chatic and its users safe. 🔐
