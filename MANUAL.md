# Chatic — User Manual

> **Languages:** **English** · [Português (Brasil)](MANUAL.pt-BR.md)
>
> This is the **end-user manual**: how to install, pair your WhatsApp, configure the AI,
> and use the tutor day to day. For a project overview see the [README](README.md).

---

## Table of contents

1. [What Chatic is](#1-what-chatic-is)
2. [Requirements](#2-requirements)
3. [Install & run](#3-install--run)
4. [First run: the admin panel](#4-first-run-the-admin-panel)
5. [Pairing WhatsApp](#5-pairing-whatsapp)
6. [Configuring the AI](#6-configuring-the-ai)
7. [Talking to the tutor](#7-talking-to-the-tutor)
8. [Chat commands](#8-chat-commands)
9. [Study modes](#9-study-modes)
10. [Groups](#10-groups)
11. [Links & PDFs](#11-links--pdfs)
12. [Voice messages (audio)](#12-voice-messages-audio)
13. [Admin tasks](#13-admin-tasks)
14. [Privacy & your data](#14-privacy--your-data)
15. [Troubleshooting](#15-troubleshooting)

---

## 1. What Chatic is

Chatic is a **private language tutor that lives inside WhatsApp**. You run it yourself (on a PC,
a mini-server, or a cheap cloud VM), pair a WhatsApp number, and then chat with an AI tutor that
corrects you, teaches vocabulary and grammar, and adapts to your level and interests — in **any**
language pair (e.g. a Portuguese speaker learning English).

There are two ways people use it:

- **Family mode (self-chat):** each person links their **own** WhatsApp as a companion device and
  talks to the tutor in their **own** "message yourself" chat, using a prefix (default `!`). No
  whitelist, no admin — pairing your own phone *is* the authorization.
- **Shared mode:** one paired number acts as the "bot". Other people DM that number; an admin
  controls who is allowed (a whitelist). This account can also serve WhatsApp **groups**.

You can run either or both at once.

---

## 2. Requirements

- **A machine to run it on** — Linux, Windows, or macOS. It is light (designed for 1 vCPU / low RAM).
- **A phone with WhatsApp** to pair (each user pairs from their own WhatsApp app).
- **An AI provider key** — Chatic needs at least one LLM. A **free** Google Gemini key works
  (see [§6](#6-configuring-the-ai)). OpenAI, Claude, or a local Ollama also work.
- **FFmpeg — optional.** Only needed for **voice** (transcribing incoming audio and speaking
  replies). Without it, the full **text** tutor works fine; Chatic just logs an install hint at
  startup and disables audio gracefully.

---

## 3. Install & run

### Option A — download a release build (recommended)

1. Grab the archive for your OS from the project's Releases page (`chatic_<version>_<os>_<arch>`).
2. Extract it. You get the `chatic` binary, a `README`, `LICENSE`/`NOTICE`, and a `.env.example`.
3. (Optional) copy `.env.example` to `.env` and adjust settings — **you don't need to put API keys
   here**; the recommended path is the web panel.
4. Run the binary:
   - Linux/macOS: `./chatic`
   - Windows: `chatic.exe`

On Debian/Ubuntu or Fedora/RHEL you can instead install the `.deb`/`.rpm`, which registers a
`systemd` service (`chatic.service`) that starts on boot.

### Option B — build from source

Requires **Go 1.20+**:

```bash
go mod tidy
go build -o chatic ./cmd/server
./chatic
```

The database is created automatically on first run under `storage/` — **a fresh install starts
with an empty, clean database** (no accounts, no history).

---

## 4. First run: the admin panel

When Chatic starts it opens a **web admin panel**. By default:

- URL: `http://localhost:3030/admin` (change the port with `PORT` in `.env`).
- On the very first boot an initial admin is seeded from `INITIAL_ADMIN_NUMBER` in `.env`
  (WhatsApp number, digits only, with country + area code, e.g. `5511999999999`). This is used
  **only** to seed the admin — afterward it lives in the database.

The panel is where you do everything that shouldn't happen in a chat: pair WhatsApp accounts,
manage API keys, edit the system prompt, manage the whitelist, and delete user data.

> **The QR code to pair WhatsApp is shown only in the panel** — never in the terminal/logs — for
> privacy.

---

## 5. Pairing WhatsApp

Open the panel → **WhatsApp Accounts** → **Add WhatsApp**. Pick a **role**:

- **Shared** — the "bot" number others DM (whitelist-gated, admin-managed, serves groups).
  There can be **at most one** shared account, and it's **optional**.
- **Personal** — a household member's own WhatsApp, talked to via self-chat. No whitelist.

Then pair using either method:

- **QR code (default):** open WhatsApp on your phone → **Settings → Linked Devices → Link a
  device** → scan the QR shown in the panel.
- **Phone code:** the panel gives you an 8-character code; in WhatsApp choose **Link with phone
  number instead** and type it.

You can promote/demote the **Shared** role later with the switch next to each account (turning one
on turns the current one off). If there is **no** shared account, the panel shows a banner — that's
fine; groups and third-party DMs are simply unavailable until you designate one, while personal
self-chats keep working.

> **Headless setup:** set `PAIR_CODE_PHONE=<number>` in `.env` to pair a shared account by phone
> code at startup without opening the panel.

### Family mode in practice

Each family member opens the panel once, adds their **own** WhatsApp as **Personal**, and scans
the QR from their own phone. From then on they just open their **"message yourself"** chat in
WhatsApp and start a line with `!` (the `SELF_CHAT_PREFIX`, configurable) to talk to the tutor.
Everything else that companion device sees (real chats, groups) is ignored by Chatic.

---

## 6. Configuring the AI

Chatic needs at least one LLM provider. Manage this in the panel under **AI Settings**.

### Getting a free key (Google Gemini)

1. Go to Google AI Studio and sign in with a Google account.
2. Create an API key (free tier is enough to start).
3. In the panel → **AI Settings** → the **Gemini** card → paste the key → **Add**.

### Providers, keys, and the primary

- Each provider (**Gemini / OpenAI / Claude**) has its **own key list**. **Key #1 is the primary**;
  any extra keys form a **round-robin pool** (Chatic rotates through them to spread rate limits).
- Add a key with the provider's form; remove a bad one with the 🗑 next to it.
- The **⭐ Set as primary** control chooses which **provider** is used first
  (`PRIMARY_LLM_PROVIDER`). If it fails, Chatic **fails over** to the others automatically
  (Gemini → OpenAI → Claude → Ollama).
- **Ollama** (local, no key) uses a base URL instead — set it in the Ollama card.

Keys are **encrypted at rest** and never written to `.env`.

### Personal keys

- In chat, a user can set their own key with `/myai <provider> <key> [model]` (exclusive to them,
  not added to the shared pool). The bot never echoes the key and asks you to delete the message.
- The **preferred** way is the panel (**🤖 IA** button on a user) so the key never touches chat
  history.

### System prompt (optional)

**AI Settings** has an editable **system prompt**. Leave it **empty** to use Chatic's built-in
tutor prompt. Click **Load default template** to start from the built-in and customize it. It
supports placeholders that are filled per user: `{IdiomaAlvo}` (target language), `{IdiomaNativo}`
(native language), `{Nivel}` (level), `{Interesses}` (interests), `{NomeProfessor}` (teacher name).

---

## 7. Talking to the tutor

The first time you message the tutor it runs a short **onboarding**:

1. Your name
2. Birth year
3. **Native language** (your support language, e.g. Portuguese)
4. **Target language** (what you want to learn, e.g. English)
5. **Level** — take a 3-question quick placement test, or skip and start at A1
6. **Teacher's name** — what you want to call your tutor (it will answer to that name)
7. **Interests / hobbies** — used to steer conversation topics

After that, just **chat naturally in your target language**. The tutor replies in context and,
when you make a mistake, adds a short **💡 Quick Tip** correction without derailing the
conversation. You earn **XP** as you practice (see `/ranking`).

Type `/restart` to redo onboarding, or `/language` to review/adjust your language settings.

---

## 8. Chat commands

All commands start with `/`. Universal ones work in a DM/self-chat:

| Command | What it does |
|---|---|
| `/help` | Lists the available commands |
| `/restart` | Restarts onboarding |
| `/language` | Review / adjust your native & target language |
| `/tips` | Suggests things you could say next, in your target language (scaffolding) |
| `/ranking` | Shows your XP / progress |
| `/myai <provider> <key> [model]` | Set your **personal** AI key (see [§6](#6-configuring-the-ai)) |
| `/forget` | **Erase all your data** (two-step: it asks you to confirm with `/forget CONFIRM`) |

Study modes and group commands are covered in the next sections.

---

## 9. Study modes

Focused, on-demand practice (each is a single reply, in addition to the corrections that happen
naturally in conversation):

| Command | What it does |
|---|---|
| `/grammar <topic>` | Explains a grammar rule in your **native** language with examples |
| `/word` | Teaches one useful word/expression for your level and interests |
| `/vocab <theme>` | A themed vocabulary list (with translations and examples) |
| `/quiz` | A short quiz based on your recent conversation (answers revealed at the end) |
| `/fix <sentence>` | Corrects one sentence and explains why (bare `/fix` corrects your last message) |

---

## 10. Groups

Chatic can join WhatsApp **groups** (served by the **shared** account only).

**Setup**

| Command | What it does |
|---|---|
| `/newgroup` | Creates a study group and gives you a join code |
| `/join <code>` | Join an existing study group |
| `/groupai <code> <provider> <key> [model]` | Set a **shared** AI key for the group |

**In a group** the tutor stays quiet unless invited:

- **Phase 1 (reactive):** `/ask <question>`, `/correct <text>`, or **@mention** the bot.
- **Phase 2 (activities):**

| Command | What it does |
|---|---|
| `/gquiz [theme]` | Posts a native WhatsApp **poll** quiz |
| `/greveal` | Reveals the answer + explanation of the last `/gquiz` |
| `/gword` | Word of the day for the group |
| `/gchallenge [theme]` | A quick group challenge |
| `/ghelp` | Lists the group commands |

Groups have a rate limit to keep AI cost bounded.

---

## 11. Links & PDFs

- **Send a link** (any `http(s)` URL) in a DM and Chatic fetches the page, extracts the readable
  text, and discusses the **real content** with you instead of guessing from the URL.
- **Send a PDF** and Chatic extracts its text (first pages) so you can study from it. Only PDFs are
  supported; scanned/image-only PDFs won't yield text (no OCR).

For safety, fetched pages and uploaded files are always treated as **reference material, never
instructions** (protection against prompt-injection), and unreachable links fail gracefully.

---

## 12. Voice messages (audio)

If **FFmpeg** is installed:

- **Send a voice message** and Chatic transcribes it, tutors on it, and can reply.
- Replies can be spoken back as audio (text-to-speech).

Without FFmpeg, sending a voice message in a DM gets a friendly "voice disabled, keep typing"
reply — the text tutor is unaffected. Temporary audio files are deleted immediately after use.

---

## 13. Admin tasks

For the **shared** account, an admin manages who is allowed. From the admin's own chat:

| Command | What it does |
|---|---|
| `/list` | List whitelisted users |
| `/add <number>` | Allow a WhatsApp number (digits only, with country/area code) |
| `/delete <number>` | **Erase** that user's data and remove them |
| `/recover <number>` | Restore a previously removed user |

The panel mirrors these (user list, add, delete) plus API-key and system-prompt management. Unknown
senders to the shared account are dropped **before** any processing.

---

## 14. Privacy & your data

Privacy is the point of Chatic. Key guarantees:

- **Message content is never logged** to the console — only metadata (like a user id).
- **Chat history is encrypted at rest** in the local database.
- **API keys are encrypted at rest** and never written to `.env`.
- **You run it** — your data stays on your machine; nothing is sold or sent to a third party beyond
  the AI provider call needed to generate a reply.
- **Right to erasure (LGPD):** `/forget` (then `/forget CONFIRM`) hard-deletes **all** of your
  data — profile, messages, and group memberships. An admin can also erase a user with `/delete`,
  and the panel has a delete-user button. Erasure is a real physical delete, not a hide.

A **release build always ships with a clean, empty database** — no paired accounts or personal data
are ever bundled into the distributed package.

---

## 15. Troubleshooting

**The panel won't open / port in use.** Another process holds the port. Change `PORT` in `.env`, or
stop the other instance.

**No QR appears when adding an account.** Wait a moment (the panel polls for it) and make sure you
picked a role. If it says expired, close and re-open **Add WhatsApp** to start a fresh pairing.

**"No shared account" banner.** Expected if you only paired personal devices. Groups and
third-party DMs need a shared account; designate one with the **Shared** switch. Personal
self-chats work regardless.

**The tutor doesn't answer.** Check **AI Settings**: you need at least one working key. If the
primary provider is down or out of quota, Chatic fails over — but if **all** providers fail you'll
get an error. Add/replace a key with the 🗑 / **Add** controls.

**Voice messages don't work.** Install **FFmpeg** and restart. Chatic logs an OS-specific install
hint at startup when it's missing.

**A message wasn't answered on the shared account.** Only whitelisted numbers are served; add the
number with `/add` or in the panel.

**Personal device isn't responding.** Make sure you're messaging **yourself** (the "message
yourself" chat) and that your line starts with the prefix (`!` by default).
