# Chatic — Private Multilingual Language Tutor (WhatsApp Bot)

**English** · [Português (BR)](README.pt-BR.md)

Chatic is a private language-learning ecosystem over WhatsApp, written in Go and designed to run with extreme efficiency on modest hardware (such as free Oracle Cloud instances with 1 CPU core and 6 GB RAM).

It supports practicing **any language the student chooses** (e.g. English, Spanish, French, German, Japanese) from their **native language** (e.g. Portuguese, Spanish, English), offering conversational practice (text and audio), gamification (XP and family rankings) and protection against wasted tokens (AI failover).

> 📖 **New here? Read the [User Manual](MANUAL.md)** — a step-by-step guide to installing, pairing WhatsApp, configuring the AI, and using every command.

---

## Quick Start (no `.env` editing needed)

You don't have to touch `.env` to get running — everything is configured in the web panel:

1. **Install** Chatic (see **Installation** below) and start it.
2. Open **http://localhost:3030/admin** and create the panel password (first access).
3. Add a **free** AI key (see the next section) under **⚙️ AI Settings** and save.
4. Open the **Pairing** tab and **scan the QR** with WhatsApp.
5. Add yourself / your family under **Users** (or enable multi-account so each person pairs their own WhatsApp).

That's it — start chatting with the tutor on WhatsApp. The `.env` file is **optional** and only for advanced/host tuning.

---

## Free AI Key (Google AI Studio) — it's free 🎉

Chatic runs great on **Google Gemini**, which has a **generous free tier** — enough for everyday family use at **no cost**.

1. Go to **https://aistudio.google.com/apikey** and sign in with a Google account.
2. Click **Create API key** and copy it (it looks like `AIza…`).
3. In the panel (**/admin → ⚙️ AI Settings**), paste it into the **Google Gemini API Key** field and **Save**.
   - The key is stored **encrypted** in the database — **never** put it in `.env` or paste it into a chat.
4. Done — the tutor immediately starts using it.

**How it works:** every message you send is forwarded to the chosen AI provider (Gemini by default) together with a tutoring system prompt; the bot has automatic **failover** (Gemini → OpenAI → Claude → Ollama), so a single provider outage or quota limit doesn't stop the lesson. Hitting the free daily limit? Add **several** Gemini keys (comma-separated) for a round-robin pool, or run a **fully local, free** model with **Ollama** (see below) — no cloud, no cost.

> 📖 More detail — multiple keys, per-user keys, Ollama — is in the [User Manual → Configuring the AI](MANUAL.md#6-configuring-the-ai).

---

## Environment Configuration (.env) — optional

The `.env` file is **optional**: Chatic boots with sensible defaults and you configure keys and users in the web panel. Edit `.env` only for advanced/host tuning (port, timeouts, the message-age guard, multi-account). If you do create one, base it on the template below.

> 🔒 **Secrets never live in `.env`.** API keys and other sensitive data are configured through the **web panel** (`/admin`) and stored **encrypted (AES‑256‑GCM)** in the SQLite database, protected by the local master key (`storage/.masterkey`, `0600`) — never in plaintext on disk. The `.env` only holds **non-sensitive** settings; the bot itself rewrites the file without keys when you save settings in the panel. You may optionally put a key in `.env` just for the initial *bootstrap* — it is migrated to the encrypted vault as soon as you save in the panel.

```env
# General settings
PORT=3030
ENV=development

# Self-Chat: talk to the bot on your own number ("Message yourself")
# Messages starting with this prefix are processed by the tutor. Leave empty to disable.
SELF_CHAT_PREFIX=!

# Primary LLM provider (options: gemini, openai, claude, ollama)
PRIMARY_LLM_PROVIDER=gemini

# API keys are NOT stored here. Configure them via the web panel (/admin) — they are
# kept encrypted (AES-GCM) in SQLite, not in .env. The lines below are optional
# bootstrap only and migrate to the encrypted vault once you save in the panel.
# GEMINI_API_KEY=
# OPENAI_API_KEY=
# CLAUDE_API_KEY=

# Local LLM configuration (Ollama)
OLLAMA_API_BASE=http://localhost:11434
OLLAMA_MODEL=llama3.2

# Limits and timeouts
LLM_TIMEOUT_SECONDS=10

# Max age (in seconds) of an incoming message still worth processing.
# On reconnect after downtime, WhatsApp replays the whole offline backlog; without
# this limit the bot would answer them all at once, flooding the chats.
# Older messages are ignored. Use 0 to disable. (default: 300)
MAX_MESSAGE_AGE_SECONDS=300

# Database
DATABASE_PATH=storage/tutor.db

# Admin WhatsApp number — used ONLY on the first boot to seed the admin.
# It is personal data: once created, the admin lives in the database (users table)
# and the bot does NOT rewrite this number to .env. Fill it only for the initial bootstrap.
# Digits only, with country + area code.
INITIAL_ADMIN_NUMBER=

# Phone-code pairing (optional): instead of scanning the QR in the panel, generate an
# 8-digit code to type into WhatsApp. Leave empty to use the QR.
PAIR_CODE_PHONE=

# Multi-account mode (optional): lets each person in the household pair their OWN
# WhatsApp and talk to the tutor via self-chat, with no whitelist and no admin.
# true = enables pairing new personal accounts from the panel. (default: false)
MULTI_ACCOUNT_ENABLED=false
```

---

## AI Integration and Local LLM (Ollama)

Besides cloud providers (Gemini, OpenAI, Claude), you can run the bot **fully local and offline** by integrating **Ollama**:

1.  **Install Ollama**: download and install from [ollama.com](https://ollama.com/) on your host (Windows, Mac or Linux).
2.  **Pull the model** you want:
    ```bash
    ollama pull llama3.2
    ```
3.  **Enable it in the bot**: in `.env`, set:
    ```env
    PRIMARY_LLM_PROVIDER=ollama
    OLLAMA_API_BASE=http://localhost:11434  # Or the Windows IP if running the bot inside WSL (e.g. http://172.x.x.x:11434)
    OLLAMA_MODEL=llama3.2
    ```

---

## Customizing the Agent (Prompts)

There are two ways to edit the bot's personality, tone and teaching behavior:

### Option A: Without recompiling (via `.env`)
Edit the `CUSTOM_SYSTEM_PROMPT` variable in `.env`. The bot dynamically substitutes the tags below based on the profile of whoever is chatting:
-   `{IdiomaAlvo}`: the language the student wants to learn (e.g. English).
-   `{IdiomaNativo}`: the student's native language (e.g. Portuguese).
-   `{Nivel}`: the CEFR proficiency level (A1 to C2).
-   `{Interesses}`: hobbies and topics the student likes.
-   `{NomeProfessor}`: the name the student chose for the tutor during onboarding.

> The placeholder tokens stay in Portuguese on purpose — they are a stable, user-facing template contract.

### Option B: Recompiling the code
Edit the structured default prompt and logic directly in `internal/tutor/engine.go`.

---

## Installation

Pick whichever method you prefer. The **text** tutor runs with **zero system dependencies**; **FFmpeg** is optional (audio only, and the installers try to set it up for you).

> After installing, open **http://localhost:3030/admin**, create the panel password (first access) and **scan the QR Code** to pair WhatsApp — the QR appears in the panel itself, not in the terminal.

### Linux / macOS — one line
```bash
curl -fsSL https://raw.githubusercontent.com/jousudo/chatic/main/install.sh | sh
```

### Windows (PowerShell) — one line
```powershell
irm https://raw.githubusercontent.com/jousudo/chatic/main/install.ps1 | iex
```

### Linux with a service (auto-start on boot) — .deb / .rpm package
Download the package from the [Releases page](https://github.com/jousudo/chatic/releases) and install:
```bash
# Debian/Ubuntu
sudo apt install ./chatic_*_amd64.deb
# Fedora/RHEL
sudo dnf install ./chatic_*_amd64.rpm
```
The package creates a hardened `systemd` service, starts it automatically and puts data in `/var/lib/chatic`. Edit `/var/lib/chatic/.env` and run `sudo systemctl restart chatic`.

### Docker
```bash
docker run -d --name chatic -p 3030:3030 -v chatic-data:/app/storage ghcr.io/jousudo/chatic:latest
```

---

## Build from Source

### Prerequisites
*   **Go** 1.20+ — only to build from source; the release packages already ship a ready-to-run binary.
*   **FFmpeg** — **optional**. Needed only for *audio* features (receiving voice messages and replying with audio). Without it, the bot logs a hint at startup and the full **text** tutor works normally. To enable audio, install FFmpeg and keep it on the PATH.
*   A phone with an active WhatsApp to scan the bot's authentication QR Code.

### Local run
1.  (Optional, for audio) install FFmpeg:
    ```bash
    # Debian/Ubuntu:
    sudo apt-get install ffmpeg
    ```
2.  Install Go dependencies:
    ```bash
    go mod tidy
    ```
3.  Build the binary:
    ```bash
    go build -o chatic ./cmd/server
    ```
4.  Run the bot:
    ```bash
    ./chatic
    ```
5.  Open **http://localhost:3030/admin**, create the panel password (first access) and **scan the QR Code on the Pairing tab** with WhatsApp — the QR appears in the panel, not in the console.

---

## Commands and Usage

> All commands are in English. Send `/help` any time to see the full list inside WhatsApp.

### General commands (for every authorized user)
*   `/help` — show the list of available commands.
*   `/restart` — redo the full setup (name, languages, level, tutor name, interests).
*   `/language <language>` — change only the language you are learning (e.g. `/language French`).
*   `/tips` — get reply suggestions in the language you are learning (useful after a tutor audio).
*   `/grammar <topic>` — explain a grammar rule with examples (e.g. `/grammar past tense`).
*   `/word` — learn a useful word/expression of the day.
*   `/vocab <theme>` — build a themed mini-vocabulary list (e.g. `/vocab travel`).
*   `/quiz` — take a quick grammar & vocabulary quiz (answer key at the end).
*   `/fix <sentence>` — get an explicit correction (or bare `/fix` to fix your last message).
*   `/ranking` — show the XP leaderboard across participants.
*   `/forget` — **permanently** delete all your data (confirm with `/forget CONFIRM`).
*   `/myai <provider> <key> [model]` — use your own personal AI provider.
*   `/newgroup <name>` — create a study group.
*   `/join <code>` — join a study group.
*   `/groupai <code> <provider> <key> [model]` — set a group's shared AI (group admins).

### Admin commands (admin numbers only)
*   `/list` — list registered users and their XP.
*   `/add <number> <name>` — add a number to the whitelist.
*   `/delete <number>` — remove a user and revoke access.
*   `/recover` — send a panel password-recovery token to the admin's WhatsApp.

### WhatsApp group commands
The bot stays silent in groups and only acts when triggered (to bound cost and avoid spam):
*   Mention the bot, or use `/ask <question>` and `/correct <phrase>` for questions and corrections.
*   `/gquiz <theme>` — create a **native poll quiz** in the group (everyone votes); `/greveal` shows the answer.
*   `/gword` — a word of the day for the group.
*   `/gchallenge <theme>` — propose a practice challenge for the group to do together.
*   `/ghelp` — show the group command list.

> Group activities are rate-limited per group (cost protection). Bot-led lessons (`/aula`) are a planned future feature.

---

## Multi-account Mode (one household, several WhatsApps)

By default the bot runs **a single shared account**: one number everyone DMs, gated by a whitelist and managed by an admin.

The **multi-account mode** (optional) lets **each person in the household pair their own WhatsApp** and talk to the tutor by **messaging themselves** (self-chat, with the `!` prefix) — **no whitelist and no admin account**. Pairing your own device is the authorization.

**How to enable:**
1. Set `MULTI_ACCOUNT_ENABLED=true` in `.env` and restart the bot.
2. In the panel (`/admin`), section **🏠 Household Accounts**, click **➕ Add WhatsApp**.
3. On the new user's phone: **WhatsApp → Linked devices → Link a device** and scan the QR.
4. Done — the person starts chatting with the tutor by messaging themselves, starting with `!` (e.g. `!hello, let's practice`).

> ⚠️ **Privacy:** once paired, the bot becomes a **companion device** of the account (like WhatsApp Web) and technically can see the device's chats. It **only acts** on the owner's own self-chat and ignores everything else (third-party DMs, groups). Even so, only pair WhatsApp accounts of people who trust the instance you host. Already-paired accounts keep working even if you turn the flag off later (it only controls pairing of **new** accounts).

---

## Privacy and Data Protection (LGPD / GDPR)

This project is **self-hosted**: each person runs their own instance. Whoever hosts is the **data controller** for that instance's users — the software provides the mechanisms to operate in compliance with LGPD (Brazil) and equivalent regulations (GDPR etc.).

**What data is processed**
- WhatsApp number, name and learning preferences (languages, level, interests, chosen tutor name) provided during onboarding.
- Conversation history with the tutor (to give context to the learning; bounded to recent messages when building the AI context).
- Optionally, a personal AI API key.

**How data is protected**
- **Access minimization:** only whitelisted numbers are processed; any other sender is dropped before any processing.
- **Encryption at rest (AES‑256‑GCM):** the API keys (system, per-user and per-group) **and the conversation content** are encrypted in the database with a local master key (`storage/.masterkey`, `0600`, kept in a file separate from the `.db`).
- **No content logging:** the console logs only metadata (e.g. user ID), never message text.
- **External content is untrusted:** text from links and documents is treated as reference material, never as instructions to the model (defense against *prompt injection*).

**Data-subject rights**
- **Right to erasure (Art. 18):** a user can send **`/forget`** (with `/forget CONFIRM`) to *permanently* delete all their data — profile, preferences, history and personal key. An admin can also delete per user via `/delete <number>` or the panel; both do a *hard delete* (not a soft delete).
- **Access/portability:** the data lives in a single SQLite file (`storage/tutor.db`) under the host's control.

**Recommendations for hosts**
- Back up `storage/.masterkey` **separately** from the `.db` (without the key the encrypted content is unrecoverable; with both together, the at-rest encryption loses its effect against a stolen backup).
- Consider OS disk encryption (BitLocker/LUKS) for defense in depth.
- Tell your users what data is processed and why (transparency — Art. 9).

> ⚠️ Notice: this document describes the available technical features and **does not constitute legal advice**. Final compliance depends on how you operate your instance.

---

## License

This project is licensed under the **Apache License 2.0** — see the `LICENSE` and `NOTICE` files for details.
