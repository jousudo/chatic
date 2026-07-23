// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"chatic/config"
	"chatic/internal/database"
	"chatic/internal/middleware"
	"chatic/internal/model"
	"chatic/internal/queue"
	"chatic/internal/repository"
	"chatic/internal/tutor"

	"sync"

	"go.mau.fi/whatsmeow"
	waproto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// defaultMaxIncomingMessageAge is the fallback staleness window used when
// MAX_MESSAGE_AGE_SECONDS is unset (e.g. in tests where config is not loaded).
const defaultMaxIncomingMessageAge = 5 * time.Minute

// maxIncomingMessageAge returns how old an incoming WhatsApp message may be to still be
// processed, driven by MAX_MESSAGE_AGE_SECONDS. On reconnect after any downtime, WhatsApp
// replays the entire offline backlog (every message the account missed while it was down);
// without this guard the bot processes and replies to all of them, flooding each chat with
// a burst of stale, out-of-context responses. Live conversation messages carry a
// near-current server timestamp and pass freely; replayed backlog is older and is dropped.
// A configured value <= 0 disables the guard (returns 0 → every message is processed).
func maxIncomingMessageAge() time.Duration {
	if config.CurrentConfig == nil {
		return defaultMaxIncomingMessageAge
	}
	secs := config.CurrentConfig.MaxMessageAgeSeconds
	if secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

type WhatsAppService struct {
	client      *whatsmeow.Client
	clientMu    sync.RWMutex
	dbContainer *sqlstore.Container
	deviceRepo  *repository.DeviceRepository
	userRepo    *repository.UserRepository
	chatRepo    *repository.ChatRepository
	groupRepo   *repository.GroupRepository
	factory     *LLMFactory
	engine      *tutor.TutorEngine
	onboarding  *OnboardingService
	tts         *TTSService

	// Multi-account mode (optional): "personal" devices paired alongside the shared
	// account. Each is a household member's personal WhatsApp, served only
	// via the owner's own self-chat. See client_multi.go.
	personal   map[string]*personalDevice // keyed by device JID
	personalMu sync.RWMutex
	pendingQR  map[string]string // QR of in-progress pairings, keyed by pending id
	pendingMu  sync.RWMutex

	// Dynamic per-JID role. Because a device can be promoted/demoted (shared<->personal)
	// at runtime from the panel, the role is resolved at event time via currentRole()
	// instead of being captured in the event-handler closure. Seeded on boot from the
	// DeviceRepository and updated on every transition. See client_multi.go.
	roleByJID map[string]deviceRole // keyed by device JID
	roleMu    sync.RWMutex
	// swapMu serializes a full promote/demote transition (which touches client/personal/
	// role state) so the "0 or 1 shared" invariant holds under concurrency.
	swapMu sync.Mutex
}

// GetClient returns the current whatsmeow client. After a full logout (Danger
// Zone → Disconnect WhatsApp), the old device is invalidated and a new
// *whatsmeow.Client is created internally — external consumers (e.g. the web panel)
// should always fetch the client here instead of holding the old pointer, which
// would be permanently stale after the swap.
func (s *WhatsAppService) GetClient() *whatsmeow.Client {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.client
}

func (s *WhatsAppService) setClient(c *whatsmeow.Client) {
	s.clientMu.Lock()
	s.client = c
	s.clientMu.Unlock()
}

func NewWhatsAppService(
	dbContainer *sqlstore.Container,
	deviceRepo *repository.DeviceRepository,
	userRepo *repository.UserRepository,
	chatRepo *repository.ChatRepository,
	groupRepo *repository.GroupRepository,
	factory *LLMFactory,
	engine *tutor.TutorEngine,
	onboarding *OnboardingService,
	tts *TTSService,
) *WhatsAppService {
	// Select the ALREADY-PAIRED shared account's device (role "shared"); on legacy
	// installs with no recorded roles it falls back to the first non-personal paired
	// device. A nil device means no shared account is paired (shared is optional): the
	// client stays nil until one is paired from the panel (AddDevice role "shared").
	var client *whatsmeow.Client
	if device := pickSharedDevice(dbContainer, deviceRepo); device != nil {
		client = whatsmeow.NewClient(device, nil)
	}

	return &WhatsAppService{
		client:      client,
		dbContainer: dbContainer,
		deviceRepo:  deviceRepo,
		userRepo:    userRepo,
		chatRepo:    chatRepo,
		groupRepo:   groupRepo,
		factory:     factory,
		engine:      engine,
		onboarding:  onboarding,
		tts:         tts,
		personal:    make(map[string]*personalDevice),
		pendingQR:   make(map[string]string),
		roleByJID:   make(map[string]deviceRole),
	}
}

// Start boots the shared account. Shared is optional: if none is paired the service stays
// idle (personal devices boot separately) until one is paired from the panel
// (Accounts → Add WhatsApp → Shared). The legacy startup QR is retired — all pairing now
// flows through the panel's per-pairing QR (see AddDevice). PAIR_CODE_PHONE still pairs a
// shared account at startup via phone code for headless setups.
func (s *WhatsAppService) Start() {
	// Seed the per-JID role map from persisted roles so events are classified
	// correctly from the first message (the fail-safe otherwise drops unknown JIDs).
	s.seedRolesFromRepo()

	client := s.GetClient()
	if client == nil {
		// No shared account is paired.
		if phonePair := os.Getenv("PAIR_CODE_PHONE"); phonePair != "" {
			s.startSharedPhonePairing(phonePair)
			return
		}
		log.Printf("No shared WhatsApp account paired — running without one. Pair a shared account from the admin panel (/admin, port %s → Accounts → Add WhatsApp → Shared) if you want group support and inbound DMs.", config.CurrentConfig.Port)
		return
	}

	// A shared account is already paired: reconnect its existing session (no QR).
	client.AddEventHandler(s.makeEventHandler(client))
	if err := client.Connect(); err != nil {
		// Do not kill the process: personal devices may still serve their owners.
		log.Printf("Failed to reconnect the shared WhatsApp session: %v", err)
		return
	}
	s.tagSharedDevice()
	log.Println("Shared account connected via pre-existing session.")
}

// startSharedPhonePairing pairs a NEW shared account at startup using a phone pairing code
// (PAIR_CODE_PHONE), for headless/CLI setups. On successful login the device is tagged
// "shared". Panel-driven pairing uses AddDevice instead.
func (s *WhatsAppService) startSharedPhonePairing(phonePair string) {
	dev := s.dbContainer.NewDevice()
	client := whatsmeow.NewClient(dev, nil)
	s.setClient(client)
	client.AddEventHandler(s.makeEventHandler(client))
	// Tag the device as shared once it finishes pairing and obtains its JID.
	client.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.PairSuccess, *events.Connected:
			if client.Store.ID != nil {
				s.tagSharedDevice()
			}
		}
	})
	if err := client.Connect(); err != nil {
		log.Printf("Failed to connect for phone pairing: %v", err)
		return
	}
	code, err := client.PairPhone(context.Background(), phonePair, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		log.Printf("Failed to generate the pairing code: %v", err)
		return
	}
	fmt.Printf("\n======================================================\n")
	fmt.Printf("WHATSAPP PAIRING CODE: %s\n", code)
	fmt.Printf("On your phone, go to: Linked Devices > Link with phone number\n")
	fmt.Printf("======================================================\n\n")
}

// Reconnect logs out and clears the current shared account, leaving the service WITHOUT a
// shared account. Re-pairing is done from the panel (Accounts → Add WhatsApp → Shared) —
// the legacy in-place QR reconnect was retired along with the startup QR. Personal devices
// are unaffected. It is the "Disconnect WhatsApp" action of the panel's Danger Zone.
func (s *WhatsAppService) Reconnect() {
	client := s.GetClient()
	if client == nil {
		log.Printf("Reconnect: no shared account is currently paired; nothing to disconnect.")
		return
	}
	// Capture the outgoing shared JID so its now-stale role record can be cleared
	// (Logout deletes the device from the whatsmeow store).
	oldJID := ""
	if client.Store != nil && client.Store.ID != nil {
		oldJID = client.Store.ID.String()
	}
	if client.IsLoggedIn() {
		_ = client.Logout(context.Background())
	} else if client.IsConnected() {
		client.Disconnect()
	}
	s.setClient(nil)
	if oldJID != "" {
		s.forgetRole(oldJID)
		if s.deviceRepo != nil {
			_ = s.deviceRepo.DeleteByJID(oldJID)
		}
	}
	log.Printf("Shared account disconnected. Pair a new one from the panel (Accounts → Add WhatsApp → Shared).")
}

// resolvePN resolves the phone number (PN) from a pair of addresses
// (primary + alternate), handling the new WhatsApp LID addressing.
// Prefers the address whose server is the phone-number server (s.whatsapp.net).
func (s *WhatsAppService) resolvePN(primary, alt types.JID) string {
	if primary.Server == types.DefaultUserServer && primary.User != "" {
		return primary.User
	}
	if alt.Server == types.DefaultUserServer && alt.User != "" {
		return alt.User
	}
	return primary.User
}

// isSelfChatMessage returns true if the message was sent by the user
// to themselves (the "Message yourself" chat).
// For text: requires the configured activation prefix (e.g. "!").
// For audio: always accepted — you cannot add a prefix to a voice message.
func (s *WhatsAppService) isSelfChatMessage(client *whatsmeow.Client, evt *events.Message) bool {
	if config.CurrentConfig.SelfChatPrefix == "" || client.Store.ID == nil {
		return false
	}
	ownNumber := client.Store.ID.User
	// Detect self-chat even with LID addressing (new WhatsApp): the chat is you
	// if the chat PN matches your number, or if the chat identity == sender.
	chatPN := s.resolvePN(evt.Info.Chat, evt.Info.RecipientAlt)
	isSelf := chatPN == ownNumber || evt.Info.Chat.User == evt.Info.Sender.User
	if !isSelf {
		return false
	}
	// Audio: accepted without a prefix
	if evt.Message.GetAudioMessage() != nil {
		return true
	}
	// Text: requires the activation prefix
	msgText := evt.Message.GetConversation()
	if msgText == "" && evt.Message.GetExtendedTextMessage() != nil {
		msgText = evt.Message.GetExtendedTextMessage().GetText()
	}
	return strings.HasPrefix(msgText, config.CurrentConfig.SelfChatPrefix)
}

// handleEvent processes events from a specific WhatsApp client. The same body serves
// both the shared account (role "shared", whitelist) and personal
// devices (role "personal", owner self-chat, auto-authorized) — the only difference is
// the authorization step below. The client that fired the event is passed
// explicitly for correct download/send routing in multi-account mode.
func (s *WhatsAppService) handleEvent(client *whatsmeow.Client, role, ownerPN string, rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		// Drop stale messages replayed from the offline backlog on reconnect (see
		// maxIncomingMessageAge). This prevents a flood of outdated replies after any
		// downtime — the guard sits before authorization/whitelist so both the shared
		// account and personal devices are protected and logs stay clean.
		if maxAge := maxIncomingMessageAge(); maxAge > 0 {
			if ts := evt.Info.Timestamp; !ts.IsZero() && time.Since(ts) > maxAge {
				return
			}
		}

		var cleanedSender string

		if role == rolePersonal {
			// Personal device = the owner's real WhatsApp, linked as a companion.
			// It sees ALL the owner's chats; for privacy and scope, we serve
			// ONLY the owner's own self-chat (with the activation prefix). Any
			// other chat (third-party DMs, groups) is ignored. There is no whitelist:
			// pairing your own device is itself the authorization.
			if !evt.Info.IsFromMe || !s.isSelfChatMessage(client, evt) {
				return
			}
			cleanedSender = ownerPN
		} else {
			// Shared account (legacy): others DM it; whitelist required.
			if evt.Info.IsFromMe {
				// Allow only self-chat messages with the activation prefix
				if !s.isSelfChatMessage(client, evt) {
					return
				}
			}

			// Resolve the sender's real phone number, handling the new WhatsApp
			// LID addressing (uses SenderAlt/PN when the Sender arrives as a LID).
			if evt.Info.IsFromMe && client.Store.ID != nil {
				// After the filter above, IsFromMe implies self-chat: the sender is the owner.
				cleanedSender = client.Store.ID.User
			} else {
				cleanedSender = s.resolvePN(evt.Info.Sender, evt.Info.SenderAlt)
			}
			cleanedSender = strings.Split(cleanedSender, ":")[0] // strip the :device suffix for safety

			// SECURITY: strict in-memory whitelist check
			if !middleware.Instance.Check(cleanedSender) {
				log.Printf("Access blocked for number: %s (Unknown)", cleanedSender)
				return
			}
		}

		// Extract text or audio from the message
		var textContent string
		var audioPath string
		var isAudio bool
		var documentPath, documentMime, documentName string
		isGroup := evt.Info.IsGroup
		var groupJID string
		var groupTriggered bool
		if isGroup {
			groupJID = evt.Info.Chat.String()
			groupTriggered = s.isBotMentioned(client, evt)
		}

		if evt.Message.GetConversation() != "" {
			textContent = evt.Message.GetConversation()
			// Strip the self-chat activation prefix (and following spaces) before processing
			if evt.Info.IsFromMe {
				textContent = strings.TrimSpace(strings.TrimPrefix(textContent, config.CurrentConfig.SelfChatPrefix))
			}
		} else if evt.Message.GetExtendedTextMessage() != nil {
			textContent = evt.Message.GetExtendedTextMessage().GetText()
			if evt.Info.IsFromMe {
				textContent = strings.TrimSpace(strings.TrimPrefix(textContent, config.CurrentConfig.SelfChatPrefix))
			}
		} else if evt.Message.GetAudioMessage() != nil {
			// Audio depends on FFmpeg. If missing, warn the user (only in DMs, to avoid
			// spamming groups) and ignore the message — the text tutor keeps working.
			if !FFmpegAvailable() {
				if !isGroup {
					s.sendMessageText(cleanedSender, "🎤 I currently can't process *voice messages* — *FFmpeg* is not installed on the server. But we can keep learning via *text*! ✍️\n\n_(If you administer this bot: install FFmpeg to enable audio.)_")
				}
				return
			}
			isAudio = true
			audioMsg := evt.Message.GetAudioMessage()
			data, err := client.Download(context.Background(), audioMsg)
			if err != nil {
				log.Printf("Error downloading audio: %v", err)
				return
			}
			// Temporarily save the .ogg
			tempOgg, err := os.CreateTemp("storage", "audio_*.ogg")
			if err != nil {
				log.Printf("Error creating temporary ogg file: %v", err)
				return
			}
			tempOgg.Write(data)
			tempOgg.Close()

			// Convert to MP3 via FFmpeg
			tempMp3Path := strings.Replace(tempOgg.Name(), ".ogg", ".mp3", 1)
			cmd := exec.Command("ffmpeg", "-y", "-i", tempOgg.Name(), tempMp3Path)
			err = cmd.Run()
			os.Remove(tempOgg.Name()) // Remove the .ogg immediately

			if err != nil {
				log.Printf("Error running FFmpeg conversion: %v. Check that FFmpeg is on PATH.", err)
				return
			}
			audioPath = tempMp3Path
			textContent = "[Voice audio]" // Initial placeholder
		} else if docMsg := evt.Message.GetDocumentMessage(); docMsg != nil {
			// Document (e.g. PDF): download to a temp file and leave text extraction
			// to the worker. The document caption becomes the student's message.
			documentMime = docMsg.GetMimetype()
			documentName = docMsg.GetFileName()
			data, err := client.Download(context.Background(), docMsg)
			if err != nil {
				log.Printf("Error downloading document: %v", err)
				return
			}
			tempDoc, err := os.CreateTemp("storage", "doc_*.bin")
			if err != nil {
				log.Printf("Error creating temporary document file: %v", err)
				return
			}
			tempDoc.Write(data)
			tempDoc.Close()
			documentPath = tempDoc.Name()
			textContent = strings.TrimSpace(docMsg.GetCaption())
			if evt.Info.IsFromMe {
				textContent = strings.TrimSpace(strings.TrimPrefix(textContent, config.CurrentConfig.SelfChatPrefix))
			}
		}

		if textContent == "" && !isAudio && documentPath == "" {
			return // Unsupported message type (e.g. image, sticker)
		}

		// DoS guardrail (OWASP LLM04): reject oversized DM text BEFORE it enters the queue or
		// reaches the LLM, bounding token cost per message. Long material belongs in a link or a
		// PDF (which we ingest and cap separately). Groups/audio/documents are exempt (audio and
		// documents are not free text; groups have their own cooldown).
		if !isGroup && audioPath == "" && documentPath == "" && len([]rune(textContent)) > maxUserInputLen {
			s.sendMessageText(cleanedSender, "✍️ That message is too long for me to handle well here. Please shorten it — or send it as a link or a PDF and I'll read it for you.")
			return
		}

		// Enqueue the processing task in the concurrent FIFO queue
		queue.GlobalQueue.Enqueue(queue.Job{
			SenderNumber:   cleanedSender,
			MessageText:    textContent,
			AudioPath:      audioPath,
			DocumentPath:   documentPath,
			DocumentMime:   documentMime,
			DocumentName:   documentName,
			IsGroup:        isGroup,
			GroupJID:       groupJID,
			GroupTriggered: groupTriggered,
			ProcessFunc:    s.ProcessWhatsAppMessage,
		})
	}
}

// ProcessWhatsAppMessage processes a user message sequentially from the FIFO queue.
func (s *WhatsAppService) ProcessWhatsAppMessage(job queue.Job) error {
	// If there is a temporary audio file, remove it at the end
	if job.AudioPath != "" {
		defer os.Remove(job.AudioPath)
	}
	// Same for a temporary document (e.g. downloaded PDF).
	if job.DocumentPath != "" {
		defer os.Remove(job.DocumentPath)
	}

	// 1. Fetch or register the user from the database (whitelist pre-validated)
	user, err := s.userRepo.GetByNumber(job.SenderNumber)
	if err != nil {
		// Create the default user in the database if not found but whitelisted
		user = &model.User{
			PhoneNumber:    job.SenderNumber,
			Name:           "Student",
			FlowState:      "INIT",
			OnboardingDone: false,
		}
		if err := s.userRepo.Create(user); err != nil {
			log.Printf("Critical error creating user %s in the database: %v", job.SenderNumber, err)
			return err
		}
	}

	// DoS guardrail (OWASP LLM04): per-user rolling rate limit on DM processing, to protect
	// the LLM/API quota from a runaway or abusive sender. Groups are exempt (own guardrails).
	if !job.IsGroup {
		if allowed, notify := userActivityAllowed(job.SenderNumber); !allowed {
			if notify {
				s.sendMessageText(user.PhoneNumber, "⏳ You're sending messages very fast. Give me a few seconds to catch up and then keep going. 🙂")
			}
			log.Printf("User %s throttled by the per-user rate limit.", user.Name)
			return nil
		}
	}

	// Auto-associate real WhatsApp groups with the StudyGroup on first message
	if job.IsGroup {
		group, err := s.groupRepo.GetByJID(job.GroupJID)
		if err != nil {
			group, err = s.groupRepo.CreateAutoGroup(job.GroupJID, user.ID)
			if err != nil {
				log.Printf("Error creating automatic group for JID %s: %v", job.GroupJID, err)
			} else {
				_ = s.groupRepo.AddMember(group.ID, user.ID, "ADMIN")
			}
		} else {
			isMember, _ := s.groupRepo.IsMember(group.ID, user.ID)
			if !isMember {
				_ = s.groupRepo.AddMember(group.ID, user.ID, "MEMBER")
			}
		}
		if group != nil {
			user.ActiveGroupID = group.ID
			_ = s.userRepo.Update(user)
		}
	}

	// GROUP PHASE 1: in WhatsApp groups, the tutor only responds when
	// explicitly triggered (bot mention, /ask or /correct). Ordinary
	// messages are ignored to bound cost/rate limit, and the reply goes to the
	// GROUP — never to the DM. DM commands (admin, onboarding, /newgroup,
	// /join) do not apply in the group context.
	if job.IsGroup {
		return s.handleGroupMessage(user, job)
	}

	// Universal commands (for all users). Handled BEFORE admin routing,
	// otherwise any admin "/cmd" would fall into handleAdminCommand and be silently
	// dropped if it were not a known admin command.
	cmdLower := strings.ToLower(strings.TrimSpace(job.MessageText))
	switch {
	case cmdLower == "/help":
		s.sendMessageText(user.PhoneNumber, s.buildHelpText(user))
		return nil
	case cmdLower == "/restart":
		user.OnboardingDone = false
		user.FlowState = "INIT"
		s.userRepo.Update(user)
		response, _ := s.onboarding.ProcessOnboarding(user, "")
		s.sendMessageText(user.PhoneNumber, response)
		return nil
	case cmdLower == "/ranking":
		s.sendRankingList(user.PhoneNumber)
		return nil
	case cmdLower == "/tips":
		s.sendResponseTips(user)
		return nil
	case cmdLower == "/forget confirm":
		name := user.Name
		s.eraseUser(user)
		s.sendMessageText(user.PhoneNumber, fmt.Sprintf("✅ Done, %s. All your personal data (profile, preferences, chat history and personal AI key) has been *permanently deleted*. 👋\n\nTo use the bot again later, ask the admin to re-add your number.", name))
		return nil
	case cmdLower == "/forget":
		s.sendMessageText(user.PhoneNumber, "⚠️ *Delete all my data* — this permanently erases your profile, preferences, chat history and personal AI key. It cannot be undone.\n\nTo confirm, reply exactly: */forget CONFIRM*")
		return nil
	case cmdLower == "/word":
		s.sendWordOfDay(user)
		return nil
	case cmdLower == "/quiz":
		s.sendQuiz(user)
		return nil
	case cmdLower == "/fix" || strings.HasPrefix(cmdLower, "/fix "):
		inline := strings.TrimSpace(job.MessageText[len("/fix"):])
		s.sendFix(user, inline)
		return nil
	case strings.HasPrefix(cmdLower, "/grammar"):
		topic := strings.TrimSpace(job.MessageText[len("/grammar"):])
		if topic == "" {
			s.sendMessageText(user.PhoneNumber, "Usage: */grammar <topic>*\nEx: /grammar present perfect")
			return nil
		}
		s.sendGrammarLesson(user, topic)
		return nil
	case strings.HasPrefix(cmdLower, "/vocab"):
		theme := strings.TrimSpace(job.MessageText[len("/vocab"):])
		if theme == "" {
			s.sendMessageText(user.PhoneNumber, "Usage: */vocab <theme>*\nEx: /vocab food and cooking")
			return nil
		}
		s.sendVocabList(user, theme)
		return nil
	case strings.HasPrefix(cmdLower, "/language"):
		novaLingua := strings.TrimSpace(job.MessageText[len("/language"):])
		if novaLingua == "" {
			s.sendMessageText(user.PhoneNumber, "Usage: */language <new language>*\nEx: /language French")
			return nil
		}
		code := parseLanguageCode(novaLingua)
		user.TargetLanguage = code
		s.userRepo.Update(user)
		s.sendMessageText(user.PhoneNumber, fmt.Sprintf("✅ Learning language changed to *%s*! Let's practice in it now. 🚀", s.engine.GetLanguageName(code)))
		return nil
	}

	// Handle administrative commands (only known admin commands are routed,
	// so as not to swallow an admin user's other commands).
	if user.IsAdmin && isAdminCommand(cmdLower) {
		return s.handleAdminCommand(user, job.MessageText)
	}

	// Study-group commands
	msgLower := cmdLower
	if strings.HasPrefix(msgLower, "/newgroup ") {
		groupName := strings.TrimSpace(job.MessageText[len("/newgroup"):])
		if groupName == "" {
			s.sendMessageText(user.PhoneNumber, "⚠️ Provide the group name: /newgroup <name>")
			return nil
		}

		inviteCode := fmt.Sprintf("JOIN-%X", time.Now().UnixNano()%0xFFFFF)
		novoGrupo := &model.StudyGroup{
			Name:       groupName,
			InviteCode: inviteCode,
			CreatorID:  user.ID,
			IsPrivate:  false,
		}

		if err := s.groupRepo.Create(novoGrupo); err != nil {
			s.sendMessageText(user.PhoneNumber, "❌ Error creating group.")
			return nil
		}

		_ = s.groupRepo.AddMember(novoGrupo.ID, user.ID, "ADMIN")
		s.sendMessageText(user.PhoneNumber, fmt.Sprintf("✅ Group *%s* created!\n\nShare this code with your friends so they can join:\n*/join %s*", groupName, inviteCode))
		return nil
	}

	if strings.HasPrefix(msgLower, "/join ") {
		code := strings.ToUpper(strings.TrimSpace(job.MessageText[len("/join"):]))

		grupo, err := s.groupRepo.GetByInviteCode(code)
		if err != nil {
			s.sendMessageText(user.PhoneNumber, "❌ Invalid or expired invite code.")
			return nil
		}

		isMember, _ := s.groupRepo.IsMember(grupo.ID, user.ID)
		if isMember {
			s.sendMessageText(user.PhoneNumber, fmt.Sprintf("⚠️ You are already a member of *%s*.", grupo.Name))
			return nil
		}

		_ = s.groupRepo.AddMember(grupo.ID, user.ID, "MEMBER")
		s.sendMessageText(user.PhoneNumber, fmt.Sprintf("🎉 You joined the group *%s*!", grupo.Name))
		return nil
	}

	if strings.HasPrefix(msgLower, "/myai ") {
		parts := strings.SplitN(job.MessageText, " ", 4)
		if len(parts) >= 3 {
			provider := parts[1]
			apiKey := parts[2]
			modelStr := ""
			if len(parts) > 3 {
				modelStr = parts[3]
			}
			user.CustomLLMProvider = provider
			if provider == "ollama" {
				user.CustomOllamaBase = apiKey // local endpoint, not a secret
			} else {
				user.CustomLLMAPIKey = EncryptSecret(apiKey) // encrypt at rest in the vault
			}
			user.CustomLLMModel = modelStr
			s.userRepo.Update(user)
			// Never echo the key. Tell the user to delete the message, since it stays
			// in the WhatsApp history (including on the shared device).
			s.sendMessageText(user.PhoneNumber, "✅ Personal AI configured (your key is stored encrypted).\n\n⚠️ *For your security, delete the message that contains your API key from this chat now.* Anyone with access to this WhatsApp can read it in the chat history.\n\n💡 Tip: you can also set your key in the web panel instead of chat.")
			return nil
		}
	}

	if strings.HasPrefix(msgLower, "/groupai ") {
		parts := strings.SplitN(job.MessageText, " ", 5)
		if len(parts) >= 4 {
			codigoGrupo := strings.ToUpper(parts[1])
			provider := parts[2]
			apiKey := parts[3]
			modelStr := ""
			if len(parts) > 4 {
				modelStr = parts[4]
			}
			grupo, err := s.groupRepo.GetByInviteCode(codigoGrupo)
			if err == nil {
				membro, err := s.groupRepo.GetMember(grupo.ID, user.ID)
				if err == nil && membro.Role == "ADMIN" {
					grupo.SharedLLMProvider = provider
					if provider == "ollama" {
						grupo.SharedOllamaBase = apiKey // local endpoint, not a secret
					} else {
						grupo.SharedLLMAPIKey = EncryptSecret(apiKey) // encrypt at rest
					}
					grupo.SharedLLMModel = modelStr
					_ = s.groupRepo.Save(grupo)
					s.sendMessageText(user.PhoneNumber, "✅ Group shared AI configured (key stored encrypted).\n\n⚠️ *Delete the message containing the API key from this chat now.*")
				} else {
					s.sendMessageText(user.PhoneNumber, "❌ Only group admins can configure the group AI.")
				}
			}
			return nil
		}
	}

	// 2. Onboarding state machine (initial interview)
	if !user.OnboardingDone {
		// Audio transcription happens well after this point, so during
		// onboarding an audio would arrive as the placeholder "[Voice audio]" and be
		// saved as an answer (e.g. becoming the student's name). Ask for text explicitly.
		if job.AudioPath != "" {
			s.sendMessageText(user.PhoneNumber, "🎤 During the initial setup, please reply by *typing* (not audio). Can you repeat your answer in text?")
			return nil
		}
		if job.DocumentPath != "" {
			s.sendMessageText(user.PhoneNumber, "📄 Let's finish the initial setup first. Afterwards you can send me documents to practice together. Can you reply by *typing*?")
			return nil
		}
		response, err := s.onboarding.ProcessOnboarding(user, sanitizeUserInput(job.MessageText))
		if err != nil {
			return err
		}
		s.sendMessageText(user.PhoneNumber, response)
		return nil
	}

	// 3. Normalize and sanitize the text; audio is processed directly by Gemini.
	// Sanitization strips control characters, caps the length, and neutralizes
	// prompt-injection triggers before the text reaches the LLM or the database.
	actualText := sanitizeUserInput(job.MessageText)
	if job.AudioPath != "" {
		actualText = "[Voice message]"
	}

	// The message sent to the LLM may differ from the text saved in history: if the
	// student shares a link, we inject the article content here, but the
	// database stores only the short original text (token/context savings).
	llmMessage := actualText

	// 3.5. If the message contains a link, try to download the article content for
	// real discussion — without this the LLM would see only the raw URL and hallucinate the topic.
	if job.AudioPath == "" {
		if link := extractFirstURL(actualText); link != "" {
			title, articleBody, ferr := fetchArticleText(context.Background(), link)
			if ferr != nil {
				log.Printf("Failed to load link from %s: %v", user.Name, ferr)
				s.sendMessageText(user.PhoneNumber, "🔗 I couldn't open that link (it may be blocked, paywalled, or offline). Tell me the topic in your own words and we'll still discuss it!")
				return nil
			}
			log.Printf("Article loaded from %s's link (%d chars)", user.Name, len(articleBody))
			llmMessage = buildArticleContext(actualText, title, articleBody)
		}
	}

	// 3.6. If the student sent a document, extract the text so we can discuss it together.
	// Today only PDF is supported; other types fail gracefully.
	if job.DocumentPath != "" {
		if !isPDF(job.DocumentMime, job.DocumentName) {
			s.sendMessageText(user.PhoneNumber, "📄 For now I can only read *PDF* documents. Send me a PDF and we'll discuss it together!")
			return nil
		}
		docText, derr := extractPDFText(job.DocumentPath)
		if derr != nil {
			log.Printf("Failed to extract PDF from %s: %v", user.Name, derr)
			s.sendMessageText(user.PhoneNumber, "📄 I couldn't read that PDF (it may be scanned as images or password-protected). If it's a scan, try sending the text instead!")
			return nil
		}
		log.Printf("PDF read from %s (%d chars)", user.Name, len(docText))
		llmMessage = buildDocumentContext(actualText, job.DocumentName, docText)
		// Store only a short reference in history, not the whole document.
		if strings.TrimSpace(actualText) == "" {
			name := job.DocumentName
			if name == "" {
				name = "PDF"
			}
			actualText = "[Document: " + name + "]"
		}
	}

	// 4. Fetch conversation history from SQLite (limit: 10 messages)
	history, err := s.chatRepo.GetRecentMessages(user.ID, "default", historyContextSize)
	if err != nil {
		history = []model.Message{}
	}

	// Fold older, pruned history back in via the rolling summary once the reply is sent.
	defer s.maybeSummarizeHistory(user)

	// 5. Build dynamic pedagogical instructions
	systemPrompt := s.engine.BuildSystemInstruction(user, s.customSystemPrompt())
	// Inject the long-term memory summary (decrypted from the vault) for continuity.
	systemPrompt += s.engine.SummaryClause(DecryptSecret(user.ConversationSummary))
	if user.FlowState == "IMITE" {
		systemPrompt += s.engine.BuildScaffoldingPrompt(user)
	}

	// 6. Multi-tenant LLM hierarchy resolution (keys decrypted from the vault)
	customParams := personalLLMParams(user)
	if customParams == nil && user.ActiveGroupID != 0 {
		if group, err := s.groupRepo.GetByID(user.ActiveGroupID); err == nil {
			customParams = groupLLMParams(group)
		}
	}

	// 7. AI response generation
	var response string
	var provider string

	if job.AudioPath != "" {
		// Multimodal path: Gemini processes the audio and replies as the tutor in one call
		log.Printf("Processing multimodal audio for user %s...", user.Name)
		response, provider, err = s.factory.GenerateResponseFromAudio(systemPrompt, history, job.AudioPath, customParams)
		if err != nil {
			// Fallback: send generic text to the LLM if Gemini is not configured
			log.Printf("Multimodal audio failed: %v. Falling back to text.", err)
			response, provider, err = s.factory.GenerateResponseWithFailover(systemPrompt, history, "[The user sent a voice message. Reply inviting them to practice by writing.]", customParams)
		}
	} else {
		response, provider, err = s.factory.GenerateResponseWithFailover(systemPrompt, history, llmMessage, customParams)
	}

	if err != nil {
		s.sendMessageText(user.PhoneNumber, "Sorry, I am facing technical difficulties with my intelligence engines right now.")
		return err
	}
	log.Printf("Response generated by %s for user %s", provider, user.Name)

	// 8. Save the user's and bot's messages to the SQLite database
	s.chatRepo.SaveMessage(&model.Message{
		UserID:  user.ID,
		Sender:  "user",
		Content: actualText,
		Type:    "text",
	})
	s.chatRepo.SaveMessage(&model.Message{
		UserID:  user.ID,
		Sender:  "bot",
		Content: response,
		Type:    "text",
	})

	// 9. Compute and add XP to the user
	xpGained := s.engine.CalculateXP(actualText, job.AudioPath != "")
	oldXP := user.XP
	s.userRepo.AddXP(user.PhoneNumber, xpGained)

	// Dynamic level calculation (smooth logarithmic curve: base 100)
	oldLevel := int(math.Floor(math.Pow(float64(oldXP)/100.0, 1.0/1.5))) + 1
	newXP := oldXP + xpGained
	newLevel := int(math.Floor(math.Pow(float64(newXP)/100.0, 1.0/1.5))) + 1

	if newLevel > oldLevel {
		s.sendMessageText(user.PhoneNumber, fmt.Sprintf("🎉 *LEVEL UP!* 🎉\nYou reached *Level %d*! Keep up the great work!", newLevel))
	}

	// 10. Split the Quick Tip from the main reply before generating the audio.
	// The Quick Tip should be read (text), not heard — it goes in a separate text bubble.
	tutorReply, quickTip := splitQuickTip(response)

	// 11. Send the reply back to WhatsApp (text and/or TTS audio)
	if job.AudioPath != "" {
		// Return only the pedagogical part as audio (without the Quick Tip)
		audioFile, err := s.tts.TextToSpeech(tutorReply, user.TargetLanguage, user.Level)
		if err == nil {
			defer os.Remove(audioFile)
			s.sendMessageAudio(user.PhoneNumber, audioFile)
			if quickTip != "" {
				s.sendMessageText(user.PhoneNumber, quickTip)
			}
			return nil
		}
		log.Printf("Failed to generate TTS audio: %v. Sending default text.", err)
	}

	s.sendMessageText(user.PhoneNumber, response)
	return nil
}

// splitQuickTip splits the LLM response into (main reply, quick tip).
// The Quick Tip is identified by the "💡 Quick Tip:" prefix the prompt instructs the LLM to use.
// Returns the clean pedagogical part and the Quick Tip (empty if there is no correction).
func splitQuickTip(response string) (tutorReply, quickTip string) {
	const marker = "💡 Quick Tip:"
	idx := strings.Index(response, marker)
	if idx == -1 {
		return strings.TrimSpace(response), ""
	}
	return strings.TrimSpace(response[:idx]), strings.TrimSpace(response[idx:])
}

// customSystemPrompt returns the operator's custom system-prompt override from the loaded
// config, or "" when none is set / config is not loaded (unit tests). It is injected into
// TutorEngine.BuildSystemInstruction so the engine stays decoupled from the global config.
func (s *WhatsAppService) customSystemPrompt() string {
	if config.CurrentConfig == nil {
		return ""
	}
	return config.CurrentConfig.CustomSystemPrompt
}

// Per-user rolling rate limit on direct-message processing (OWASP LLM04 — Model DoS).
// Bounds how many messages a single student can push through the LLM/API within a window,
// protecting the shared API quota from a runaway or abusive sender. Groups have their own
// guardrails (groupOnCooldown + groupActivityAllowed) and are exempt here.
var (
	userRateMu     sync.Mutex
	userRateHits   = make(map[string][]time.Time)
	userRateNotify = make(map[string]time.Time)
	userRateLimit  = 12
	userRateWindow = 30 * time.Second
)

// userActivityAllowed enforces the per-user rolling rate limit. It returns (allowed, notify):
// allowed is false once the user exceeds userRateLimit messages within userRateWindow; notify
// is true only the first time the user is throttled in the current window, so the "slow down"
// reply is sent once instead of on every dropped message.
func userActivityAllowed(number string) (allowed bool, notify bool) {
	userRateMu.Lock()
	defer userRateMu.Unlock()
	now := time.Now()
	cutoff := now.Add(-userRateWindow)
	kept := userRateHits[number][:0] // reuse the backing array (safe under the lock)
	for _, t := range userRateHits[number] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= userRateLimit {
		userRateHits[number] = kept
		if last, ok := userRateNotify[number]; !ok || now.Sub(last) >= userRateWindow {
			userRateNotify[number] = now
			return false, true
		}
		return false, false
	}
	userRateHits[number] = append(kept, now)
	return true, false
}

// --- PHASE 1: Group modes (see Section 16 of completo.md) ---

var (
	groupCooldown       = make(map[string]time.Time)
	groupCooldownMu     sync.Mutex
	groupCooldownWindow = 3 * time.Second
)

// groupOnCooldown limits how often each group can trigger, to bound cost
// and avoid floods. Returns true if the group triggered the tutor within the window.
func groupOnCooldown(jid string) bool {
	groupCooldownMu.Lock()
	defer groupCooldownMu.Unlock()
	now := time.Now()
	if last, ok := groupCooldown[jid]; ok && now.Sub(last) < groupCooldownWindow {
		return true
	}
	groupCooldown[jid] = now
	return false
}

// isBotMentioned returns true if the bot's number is in the message's mention list.
func (s *WhatsAppService) isBotMentioned(client *whatsmeow.Client, evt *events.Message) bool {
	if client.Store.ID == nil {
		return false
	}
	ext := evt.Message.GetExtendedTextMessage()
	if ext == nil || ext.GetContextInfo() == nil {
		return false
	}
	botNumber := strings.Split(client.Store.ID.User, ":")[0]
	for _, jid := range ext.GetContextInfo().GetMentionedJID() {
		mentioned := strings.Split(strings.Split(jid, "@")[0], ":")[0]
		if mentioned == botNumber {
			return true
		}
	}
	return false
}

// handleGroupMessage implements Phase 1: replies in the group only when triggered.
func (s *WhatsAppService) handleGroupMessage(user *model.User, job queue.Job) error {
	text := strings.TrimSpace(job.MessageText)
	lower := strings.ToLower(text)

	var currentMessage string
	triggered := job.GroupTriggered // native bot mention

	switch {
	case strings.HasPrefix(lower, "/correct"):
		triggered = true
		phrase := sanitizeUserInput(strings.TrimSpace(text[len("/correct"):]))
		if phrase == "" {
			s.sendToGroup(job.GroupJID, "✏️ Usage: /correct <phrase for me to correct>")
			return nil
		}
		currentMessage = fmt.Sprintf("Correct this phrase written by a student, in a short and friendly way, and briefly explain the mistake: \"%s\"", phrase)
	case strings.HasPrefix(lower, "/ask"):
		triggered = true
		q := sanitizeUserInput(strings.TrimSpace(text[len("/ask"):]))
		if q == "" {
			s.sendToGroup(job.GroupJID, "❓ Usage: /ask <your question for the tutor>")
			return nil
		}
		currentMessage = q
	case strings.HasPrefix(lower, "/gquiz"):
		// Phase 2 activity: native poll quiz (self-contained, has its own rate limit).
		s.sendGroupQuiz(user, job.GroupJID, strings.TrimSpace(text[len("/gquiz"):]))
		return nil
	case strings.HasPrefix(lower, "/greveal"):
		s.revealGroupQuiz(job.GroupJID)
		return nil
	case strings.HasPrefix(lower, "/gword"):
		s.sendGroupWord(user, job.GroupJID)
		return nil
	case strings.HasPrefix(lower, "/gchallenge"):
		s.sendGroupChallenge(user, job.GroupJID, strings.TrimSpace(text[len("/gchallenge"):]))
		return nil
	case strings.HasPrefix(lower, "/ghelp"):
		s.sendGroupHelp(job.GroupJID)
		return nil
	default:
		if !triggered {
			return nil // ordinary group message: stay silent (does not trigger the tutor)
		}
		currentMessage = sanitizeUserInput(text) // mentioned: use the whole message as the question
		if currentMessage == "" {
			currentMessage = "Introduce yourself briefly as the group's language tutor and invite members to practice."
		}
	}

	// Cost guardrail: per-group cooldown
	if groupOnCooldown(job.GroupJID) {
		log.Printf("Group %s is on cooldown; trigger ignored.", job.GroupJID)
		return nil
	}

	// Resolve the LLM prioritizing the group's shared AI; Phase 1 is stateless
	// (no shared history) to bound cost and simplify the context.
	customParams := s.resolveGroupLLM(user)
	systemPrompt := s.engine.BuildSystemInstruction(user, s.customSystemPrompt())

	response, provider, err := s.factory.GenerateResponseWithFailover(systemPrompt, []model.Message{}, currentMessage, customParams)
	if err != nil {
		s.sendToGroup(job.GroupJID, "I'm having technical difficulties right now. Please try again in a moment. 🙏")
		return err
	}
	log.Printf("Group response generated by %s (group %s)", provider, job.GroupJID)
	s.sendToGroup(job.GroupJID, response)
	return nil
}

// personalLLMParams builds the user's personal AI params, decrypting
// the API key at rest. Returns nil if the user has not configured a personal AI.
func personalLLMParams(user *model.User) *LLMConfigParams {
	if user.CustomLLMProvider == "" {
		return nil
	}
	return &LLMConfigParams{
		Provider: user.CustomLLMProvider,
		Model:    user.CustomLLMModel,
		APIKey:   DecryptSecret(user.CustomLLMAPIKey),
		BaseURL:  user.CustomOllamaBase,
	}
}

// groupLLMParams builds a group's shared AI params, decrypting
// the key at rest. Returns nil if the group has not configured a shared AI.
func groupLLMParams(group *model.StudyGroup) *LLMConfigParams {
	if group == nil || group.SharedLLMProvider == "" {
		return nil
	}
	return &LLMConfigParams{
		Provider: group.SharedLLMProvider,
		Model:    group.SharedLLMModel,
		APIKey:   DecryptSecret(group.SharedLLMAPIKey),
		BaseURL:  group.SharedOllamaBase,
	}
}

// resolveGroupLLM picks the AI for group replies: first the group's shared
// AI, then the personal AI of whoever triggered it, otherwise the system default provider.
func (s *WhatsAppService) resolveGroupLLM(user *model.User) *LLMConfigParams {
	if user.ActiveGroupID != 0 {
		if group, err := s.groupRepo.GetByID(user.ActiveGroupID); err == nil {
			if p := groupLLMParams(group); p != nil {
				return p
			}
		}
	}
	return personalLLMParams(user)
}

// sendToGroup sends a text message to a WhatsApp group's JID.
func (s *WhatsAppService) sendToGroup(groupJID string, text string) {
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		log.Printf("Invalid group JID %s: %v", groupJID, err)
		return
	}
	// Groups always belong to the shared account (personal devices only serve
	// the owner's self-chat, never groups). When no shared account is paired,
	// groups are unavailable — log and skip gracefully.
	client := s.GetClient()
	if client == nil {
		log.Printf("Group message skipped for %s: no shared account is paired.", groupJID)
		return
	}
	_, err = client.SendMessage(context.Background(), jid, &waproto.Message{
		Conversation: &text,
	})
	if err != nil {
		log.Printf("Error sending message to group %s: %v", groupJID, err)
	}
}

// isAdminCommand reports whether the text starts with a known administrative command.
// Used to route only those commands to handleAdminCommand, without swallowing the rest.
func isAdminCommand(cmdLower string) bool {
	return cmdLower == "/list" ||
		strings.HasPrefix(cmdLower, "/add ") ||
		strings.HasPrefix(cmdLower, "/delete ") ||
		cmdLower == "/recover"
}

func (s *WhatsAppService) handleAdminCommand(admin *model.User, text string) error {
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/list":
		users, _ := s.userRepo.ListAll()
		var sb strings.Builder
		sb.WriteString("📋 *Whitelist users:*\n\n")
		for _, u := range users {
			adminStr := ""
			if u.IsAdmin {
				adminStr = " [ADMIN]"
			}
			sb.WriteString(fmt.Sprintf("- ID %d: %s (%s)%s - XP: %d\n", u.ID, u.Name, u.PhoneNumber, adminStr, u.XP))
		}
		s.sendMessageText(admin.PhoneNumber, sb.String())
	case "/add":
		if len(parts) < 3 {
			s.sendMessageText(admin.PhoneNumber, "Usage: /add <number_no_spaces> <name>")
			return nil
		}
		num := sanitizePhone(parts[1])
		if num == "" {
			s.sendMessageText(admin.PhoneNumber, "Invalid number. Use only digits (country code + area + number).")
			return nil
		}
		name := sanitizeUserInput(strings.Join(parts[2:], " "))
		newUser := &model.User{
			PhoneNumber:    num,
			Name:           name,
			OnboardingDone: false,
			FlowState:      "INIT",
		}
		s.userRepo.Create(newUser)
		middleware.Instance.Add(num) // Update the in-memory whitelist
		s.sendMessageText(admin.PhoneNumber, fmt.Sprintf("✅ User %s (%s) added to the whitelist.", name, num))
	case "/delete":
		if len(parts) < 2 {
			s.sendMessageText(admin.PhoneNumber, "Usage: /delete <number>")
			return nil
		}
		num := sanitizePhone(parts[1])
		user, err := s.userRepo.GetByNumber(num)
		if err == nil {
			// Full erasure (hard delete): profile, messages, and group memberships.
			s.eraseUser(user)
			s.sendMessageText(admin.PhoneNumber, fmt.Sprintf("❌ User (%s) and all their data were permanently removed.", num))
		} else {
			s.sendMessageText(admin.PhoneNumber, "User not found.")
		}
	case "/recover":
		// Generate a panel password-recovery token and send it directly to the
		// WhatsApp of the admin who triggered the command. A strong authentication channel
		// (only whoever controls that WhatsApp receives the token), so a reset done with
		// this token does NOT wipe the data — unlike the public /recover flow
		// via browser, whose token only appears in the local console.
		var adminAccount model.AdminAccount
		if err := database.DB.First(&adminAccount).Error; err != nil {
			s.sendMessageText(admin.PhoneNumber, "No panel account found to recover.")
			return nil
		}
		token := GenerateSecureToken()
		adminAccount.ResetToken = token
		adminAccount.ResetExpiry = time.Now().Add(15 * time.Minute)
		adminAccount.ResetTrusted = true
		if err := database.DB.Save(&adminAccount).Error; err != nil {
			s.sendMessageText(admin.PhoneNumber, "Error generating recovery token.")
			return nil
		}
		s.sendMessageText(admin.PhoneNumber, fmt.Sprintf(
			"🔑 *Panel password recovery*\n\nEmail: %s\nToken: %s\n\nUse it in /recover on the web panel. Valid for 15 minutes.",
			adminAccount.Email, token,
		))
	}
	return nil
}

// buildHelpText builds the list of available commands; admin commands only appear
// for administrators.
func (s *WhatsAppService) buildHelpText(user *model.User) string {
	var sb strings.Builder
	sb.WriteString("🤖 *Available commands:*\n\n")
	sb.WriteString("• */help* — show this list\n")
	sb.WriteString("• */restart* — redo the full setup (name, languages, level, teacher name, interests)\n")
	sb.WriteString("• */language <lang>* — change only the language you're learning (ex: /language French)\n")
	sb.WriteString("• */tips* — get suggested replies in the language you're learning\n")
	sb.WriteString("• */grammar <topic>* — explain a grammar rule with examples (ex: /grammar past tense)\n")
	sb.WriteString("• */word* — learn a useful word of the day\n")
	sb.WriteString("• */vocab <theme>* — build vocabulary on a theme (ex: /vocab travel)\n")
	sb.WriteString("• */quiz* — take a quick grammar & vocabulary quiz\n")
	sb.WriteString("• */fix <sentence>* — get an explicit correction (or bare */fix* to fix your last message)\n")
	sb.WriteString("• */ranking* — see the XP leaderboard\n")
	sb.WriteString("• */forget* — permanently delete all your data (privacy)\n")
	sb.WriteString("• */myai <provider> <key> [model]* — use your own AI provider\n")
	sb.WriteString("• */newgroup <name>* — create a study group\n")
	sb.WriteString("• */join <code>* — join a study group\n")
	sb.WriteString("• */groupai <code> <provider> <key> [model]* — set a group's shared AI (group admins)\n")
	if user.IsAdmin {
		sb.WriteString("\n👑 *Admin commands:*\n")
		sb.WriteString("• */list* — list whitelisted users\n")
		sb.WriteString("• */add <number> <name>* — add a user to the whitelist\n")
		sb.WriteString("• */delete <number>* — remove a user\n")
		sb.WriteString("• */recover* — get a web-panel password reset token\n")
	}
	sb.WriteString("\n🔗 *Share a news link* and I'll read the article so we can discuss it together.\n")
	sb.WriteString("📄 *Send a PDF* and I'll read it so we can talk about it and practice.\n")
	sb.WriteString("\n💬 In a WhatsApp group, mention the bot or send */ghelp* to see the group commands (quiz polls, word of the day, challenges).")
	return sb.String()
}

// sendResponseTips generates and sends reply suggestions in the language the student is
// learning, based on the recent conversation (/tips command). Useful after a tutor
// audio, when the student does not know how to reply.
func (s *WhatsAppService) sendResponseTips(user *model.User) {
	history, err := s.chatRepo.GetRecentMessages(user.ID, "default", historyContextSize)
	if err != nil || len(history) == 0 {
		s.sendMessageText(user.PhoneNumber, "💡 Start a conversation first (send a message or a voice note), then use /tips to get suggested replies.")
		return
	}

	systemPrompt := s.engine.BuildSystemInstruction(user, s.customSystemPrompt()) + s.engine.BuildScaffoldingPrompt(user)

	customParams := personalLLMParams(user)

	response, _, err := s.factory.GenerateResponseWithFailover(
		systemPrompt, history,
		"[Give me 3 short, natural replies I could send right now, in the language I'm learning, each followed by a translation in my native language.]",
		customParams,
	)
	if err != nil {
		s.sendMessageText(user.PhoneNumber, "Sorry, I couldn't generate suggestions right now. Please try again in a moment.")
		return
	}
	s.sendMessageText(user.PhoneNumber, response)
}

// resolveLLMParams resolves the LLM params for a user following the
// personal → active-group hierarchy (keys decrypted from the vault). Returns
// nil when it falls back to the system LLM (.env).
// Context-window bounds (token & DB footprint optimization for free-tier hosts).
const (
	historyContextSize  = 10 // recent messages sent to the LLM as immediate context
	summaryTriggerCount = 30 // stored messages that trigger a summarize + prune pass
	summaryKeepRecent   = 10 // recent messages kept raw after a prune (must be < trigger)
)

// maybeSummarizeHistory keeps a user's stored history bounded. Once the message count
// passes summaryTriggerCount, it folds the oldest messages (all but the most recent
// summaryKeepRecent) into a rolling summary via one LLM call, saves that summary encrypted
// at rest on the user, and hard-deletes the folded messages. This bounds both token usage
// (LLM context = summary + last N) and DB size. Best-effort and idempotent: any failure is
// logged (metadata only) and retried on the next pass — it never blocks the conversation.
func (s *WhatsAppService) maybeSummarizeHistory(user *model.User) {
	total, err := s.chatRepo.CountMessages(user.ID, "default")
	if err != nil || total <= summaryTriggerCount {
		return
	}
	toFold := int(total) - summaryKeepRecent
	if toFold <= 0 {
		return
	}
	old, err := s.chatRepo.GetOldestMessages(user.ID, "default", toFold)
	if err != nil || len(old) == 0 {
		return
	}

	summaryPrompt := s.engine.BuildSummaryPrompt(user, DecryptSecret(user.ConversationSummary))
	newSummary, _, err := s.factory.GenerateResponseWithFailover(
		summaryPrompt, old, "[Produce the updated summary now.]", s.resolveLLMParams(user),
	)
	if err != nil {
		log.Printf("Summary skipped for user %d: %v", user.ID, err)
		return
	}
	newSummary = strings.TrimSpace(newSummary)
	if newSummary == "" {
		return
	}

	// Re-read the record so Update (a full Save) never clobbers concurrent changes,
	// then persist the summary encrypted at rest and prune the folded messages.
	fresh, err := s.userRepo.GetByID(user.ID)
	if err != nil {
		return
	}
	fresh.ConversationSummary = EncryptSecret(newSummary)
	if err := s.userRepo.Update(fresh); err != nil {
		log.Printf("Summary save failed for user %d: %v", user.ID, err)
		return
	}
	maxID := old[len(old)-1].ID
	if err := s.chatRepo.PruneMessagesUpTo(user.ID, "default", maxID); err != nil {
		log.Printf("History prune failed for user %d: %v", user.ID, err)
		return
	}
	log.Printf("Summarized %d messages for user %d (kept %d recent)", len(old), user.ID, summaryKeepRecent)
}

func (s *WhatsAppService) resolveLLMParams(user *model.User) *LLMConfigParams {
	if p := personalLLMParams(user); p != nil {
		return p
	}
	if user.ActiveGroupID != 0 {
		if group, err := s.groupRepo.GetByID(user.ActiveGroupID); err == nil {
			return groupLLMParams(group)
		}
	}
	return nil
}

// runStudyPrompt runs a study command (stateless): builds the LLM call
// with the mode's systemPrompt and sends the reply. withHistory includes the recent
// conversation as context (used by /quiz); the other modes need no history.
func (s *WhatsAppService) runStudyPrompt(user *model.User, systemPrompt, trigger string, withHistory bool) {
	var history []model.Message
	if withHistory {
		if h, err := s.chatRepo.GetRecentMessages(user.ID, "default", historyContextSize); err == nil {
			history = h
		}
	}
	response, _, err := s.factory.GenerateResponseWithFailover(systemPrompt, history, trigger, s.resolveLLMParams(user))
	if err != nil {
		s.sendMessageText(user.PhoneNumber, "Sorry, I couldn't do that right now. Please try again in a moment.")
		return
	}
	s.sendMessageText(user.PhoneNumber, response)
}

// sendGrammarLesson explains a grammar rule requested by the student (/grammar command).
func (s *WhatsAppService) sendGrammarLesson(user *model.User, topic string) {
	topic = sanitizeUserInput(topic)
	trigger := fmt.Sprintf("[Grammar topic the student asked about: %s]", topic)
	s.runStudyPrompt(user, s.engine.BuildGrammarPrompt(user), trigger, false)
}

// sendWordOfDay teaches a useful word of the day (/word command).
func (s *WhatsAppService) sendWordOfDay(user *model.User) {
	s.runStudyPrompt(user, s.engine.BuildWordOfDayPrompt(user), "[Teach the word of the day now.]", false)
}

// sendVocabList builds a themed vocabulary mini-list (/vocab command).
func (s *WhatsAppService) sendVocabList(user *model.User, theme string) {
	theme = sanitizeUserInput(theme)
	trigger := fmt.Sprintf("[Vocabulary theme the student asked about: %s]", theme)
	s.runStudyPrompt(user, s.engine.BuildVocabPrompt(user), trigger, false)
}

// sendQuiz generates a short grammar/vocabulary quiz (/quiz command).
func (s *WhatsAppService) sendQuiz(user *model.User) {
	s.runStudyPrompt(user, s.engine.BuildQuizPrompt(user), "[Generate the quiz now based on the recent conversation.]", true)
}

// sendFix gives on-demand explicit correction of a sentence (/fix command). With an inline
// argument it corrects that text; bare "/fix" corrects the student's last practice message
// from history. It does not continue the conversation — it is a focused correction.
func (s *WhatsAppService) sendFix(user *model.User, inline string) {
	target := sanitizeUserInput(strings.TrimSpace(inline))
	if target == "" {
		// Fall back to the student's most recent practice message.
		if history, err := s.chatRepo.GetRecentMessages(user.ID, "default", historyContextSize); err == nil {
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Sender == "user" {
					target = sanitizeUserInput(strings.TrimSpace(history[i].Content))
					break
				}
			}
		}
	}
	if target == "" {
		s.sendMessageText(user.PhoneNumber, "✏️ Send me a sentence first, or use */fix <sentence>*, and I'll correct it for you.")
		return
	}
	trigger := fmt.Sprintf("[Sentence the student wants corrected: %s]", target)
	s.runStudyPrompt(user, s.engine.BuildFixPrompt(user), trigger, false)
}

// eraseUser permanently deletes all of a user's personal data
// (messages, group memberships, and the profile) and removes them from the in-memory
// whitelist. Implements the right to erasure (LGPD Art. 18). Hard delete —
// no soft-delete, which would leave the data physically in the database.
func (s *WhatsAppService) eraseUser(user *model.User) {
	_ = s.chatRepo.PurgeUser(user.ID)
	_ = s.groupRepo.PurgeMemberships(user.ID)
	_ = s.userRepo.PurgeByID(user.ID)
	middleware.Instance.Remove(user.PhoneNumber)
	log.Printf("Data for user ID %d permanently erased.", user.ID)
}

func (s *WhatsAppService) sendRankingList(to string) {
	users, _ := s.userRepo.ListAll()
	var sb strings.Builder
	sb.WriteString("🏆 *Family Leaderboard* 🏆\n\n")
	medals := []string{"🥇", "🥈", "🥉"}
	for i, u := range users {
		medal := "👤"
		if i < len(medals) {
			medal = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %d. %s - %d XP (%s)\n", medal, i+1, u.Name, u.XP, u.Level))
	}
	s.sendMessageText(to, sb.String())
}

func (s *WhatsAppService) sendMessageText(to string, text string) {
	jid := types.NewJID(to, types.DefaultUserServer)
	// Route through the correct client: if "to" is a personal device's owner, reply
	// through their own device (self-chat); otherwise through the shared account.
	client := s.clientFor(to)
	if client == nil {
		// No shared account paired and "to" is not a personal owner: nowhere to send.
		log.Printf("Text reply skipped: no client available for recipient.")
		return
	}
	_, err := client.SendMessage(context.Background(), jid, &waproto.Message{
		Conversation: &text,
	})
	if err != nil {
		log.Printf("Error sending message to %s: %v", to, err)
	}
}

func (s *WhatsAppService) sendMessageAudio(to string, audioPath string) {
	// Defense in depth: without FFmpeg there is no way to convert to Opus. In practice
	// this path is not reached (audio input is already blocked without FFmpeg).
	if !FFmpegAvailable() {
		log.Printf("sendMessageAudio: FFmpeg unavailable; audio reply skipped.")
		return
	}
	jid := types.NewJID(to, types.DefaultUserServer)
	client := s.clientFor(to) // recipient-based routing (see sendMessageText)
	if client == nil {
		log.Printf("Audio reply skipped: no client available for recipient.")
		return
	}

	// Convert MP3 to Opus (.ogg) via FFmpeg
	tempOggFile, err := os.CreateTemp("storage", "send_*.ogg")
	if err != nil {
		log.Printf("Error creating temporary outbound ogg file: %v", err)
		return
	}
	tempOgg := tempOggFile.Name()
	tempOggFile.Close()
	defer os.Remove(tempOgg)

	cmd := exec.Command("ffmpeg", "-y", "-i", audioPath, "-c:a", "libopus", tempOgg)
	err = cmd.Run()
	if err != nil {
		log.Printf("Error converting outbound audio to ogg: %v", err)
		return
	}

	oggData, err := os.ReadFile(tempOgg)
	if err != nil {
		log.Printf("Error reading temporary Ogg file: %v", err)
		return
	}

	// Upload the audio to WhatsApp's servers
	resp, err := client.Upload(context.Background(), oggData, whatsmeow.MediaAudio)
	if err != nil {
		log.Printf("Error uploading audio: %v", err)
		return
	}

	audioMsg := &waproto.AudioMessage{
		URL:           &resp.URL,
		DirectPath:    &resp.DirectPath,
		MediaKey:      resp.MediaKey,
		Mimetype:      proto.String("audio/ogg; codecs=opus"),
		FileLength:    proto.Uint64(uint64(len(oggData))),
		FileSHA256:    resp.FileSHA256,
		FileEncSHA256: resp.FileEncSHA256,
	}

	_, err = client.SendMessage(context.Background(), jid, &waproto.Message{
		AudioMessage: audioMsg,
	})
	if err != nil {
		log.Printf("Error sending audio to %s: %v", to, err)
	}
}
