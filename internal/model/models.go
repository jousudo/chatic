// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package model

import (
	"time"

	"gorm.io/gorm"
)

// User is a registered, authorized (whitelisted) student in the ecosystem.
// NOTE: struct tags pin the physical column/table names so renaming these Go identifiers
// to English never changes the SQLite schema (existing databases keep working, no migration).
type User struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	Name                string         `gorm:"column:nome;size:100;not null" json:"nome"`
	PhoneNumber         string         `gorm:"column:numero_wpp;size:50;uniqueIndex;not null" json:"numero_wpp"` // Format: country code + area code + number
	IsAdmin             bool           `gorm:"default:false" json:"is_admin"`
	Level               string         `gorm:"column:nivel;size:5;default:'A1'" json:"nivel"` // A1, A2, B1, B2, C1, C2
	NativeLanguage      string         `gorm:"column:idioma_nativo;size:20;default:'pt-BR'" json:"idioma_nativo"`
	TargetLanguage      string         `gorm:"column:idioma_alvo;size:20;default:'en'" json:"idioma_alvo"`
	Interests           string         `gorm:"column:interesses;type:text" json:"interesses"`
	BirthYear           int            `gorm:"column:ano_nascimento;default:0" json:"-"`             // year of birth; 0 = unspecified. Sensitive (minors) — never serialized. Drives age-based pedagogy/safety.
	TeacherName         string         `gorm:"column:nome_professor;size:100" json:"nome_professor"` // Name the student chose for the tutor during onboarding
	XP                  int            `gorm:"default:0" json:"xp"`
	OnboardingDone      bool           `gorm:"column:onboarding_ok;default:false" json:"onboarding_ok"`
	FlowState           string         `gorm:"column:estado_flow;size:50;default:'INIT'" json:"estado_flow"`
	PublicProfile       bool           `gorm:"column:perfil_publico;default:false" json:"perfil_publico"`
	SpecialRole         string         `gorm:"column:cargo_especial;size:100" json:"cargo_especial"`
	Location            string         `gorm:"column:localizacao;size:100" json:"localizacao"`
	ShareXP             bool           `gorm:"column:compartilhar_xp;default:true" json:"compartilhar_xp"`
	ShareInterests      bool           `gorm:"column:compartilhar_interesses;default:true" json:"compartilhar_interesses"`
	ShareLanguages      bool           `gorm:"column:compartilhar_idiomas;default:true" json:"compartilhar_idiomas"`
	ShareContact        bool           `gorm:"column:compartilhar_contato;default:true" json:"compartilhar_contato"`
	CustomLLMProvider   string         `gorm:"size:50" json:"custom_llm_provider"`
	CustomLLMModel      string         `gorm:"size:100" json:"custom_llm_model"`
	CustomLLMAPIKey     string         `gorm:"size:255" json:"-"`                         // secret: never serialize in JSON responses
	CustomOllamaBase    string         `gorm:"size:255" json:"-"`                         // private endpoint: never serialize in JSON responses
	ConversationSummary string         `gorm:"column:resumo_conversa;type:text" json:"-"` // rolling LLM summary of older, pruned messages (long-term context continuity + token/DB bound). Encrypted at rest (LGPD). Never serialized.
	ActiveGroupID       uint           `json:"active_group_id"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// TableName pins the physical table name so the English type rename keeps the legacy table.
func (User) TableName() string { return "usuarios" }

// Age returns the student's approximate age from BirthYear (0 if unspecified).
// Year-only, so it may be off by up to a year — enough to drive age-band pedagogy/safety.
func (u *User) Age() int {
	if u.BirthYear <= 0 {
		return 0
	}
	return time.Now().Year() - u.BirthYear
}

// Message is the structured conversation history for each user.
type Message struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"column:usuario_id;index;not null" json:"usuario_id"`
	Sender    string         `gorm:"size:20;not null" json:"sender"` // "user" or "bot"
	Content   string         `gorm:"column:conteudo;type:text;not null" json:"conteudo"`
	Type      string         `gorm:"column:tipo;size:20;default:'text'" json:"tipo"` // "text" or "audio"
	Session   string         `gorm:"column:sessao;size:100;default:'default'" json:"sessao"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// TableName pins the legacy (GORM-pluralized) table name.
func (Message) TableName() string { return "mensagems" }

// AdminAccount holds the encrypted credentials of the panel administrator.
type AdminAccount struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	Email         string         `gorm:"size:100;uniqueIndex;not null" json:"email"`
	PasswordHash  string         `gorm:"size:255;not null" json:"-"` // password hash: never serialize
	SessionToken  string         `gorm:"size:255" json:"-"`          // session token: never serialize
	SessionExpiry time.Time      `json:"session_expiry"`             // server-side session expiry
	ResetToken    string         `gorm:"size:255" json:"-"`          // reset token: never serialize
	ResetExpiry   time.Time      `json:"reset_expiry"`
	ResetTrusted  bool           `gorm:"default:false" json:"-"` // token generated via the admin's WhatsApp (strong channel): reset does not wipe data
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// SystemConfig stores settings and API keys encrypted at rest via AES-GCM.
type SystemConfig struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Provider      string    `gorm:"size:50" json:"provider"`
	EncryptedKeys string    `gorm:"type:text" json:"encrypted_keys"` // Encrypted keys as JSON
	OllamaBase    string    `gorm:"size:255" json:"ollama_base"`
	OllamaModel   string    `gorm:"size:100" json:"ollama_model"`
	TimeoutSecs   int       `json:"timeout_secs"`
	SystemPrompt  string    `gorm:"type:text" json:"system_prompt"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// StudyGroup is a conversation club or guild created by students.
type StudyGroup struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	Name              string         `gorm:"column:nome;size:100;not null" json:"nome"`
	Description       string         `gorm:"column:descricao;type:text" json:"descricao"`
	InviteCode        string         `gorm:"column:codigo_convite;size:20;uniqueIndex;not null" json:"codigo_convite"`
	CreatorID         uint           `gorm:"column:criador_id;not null" json:"criador_id"`
	MemberLimit       int            `gorm:"column:limite_membros;default:0" json:"limite_membros"` // 0 = no limit
	IsPrivate         bool           `gorm:"default:false" json:"is_private"`
	WhatsAppJID       string         `gorm:"size:100;index" json:"whatsapp_jid"`
	SharedLLMProvider string         `gorm:"size:50" json:"shared_llm_provider"`
	SharedLLMModel    string         `gorm:"size:100" json:"shared_llm_model"`
	SharedLLMAPIKey   string         `gorm:"size:255" json:"-"` // secret: never serialize in JSON responses
	SharedOllamaBase  string         `gorm:"size:255" json:"-"` // private endpoint: never serialize in JSON responses
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// TableName pins the legacy table name.
func (StudyGroup) TableName() string { return "grupo_estudos" }

// WhatsAppDevice maps a paired WhatsApp device (whatsmeow store) to the role it
// plays in this instance. It enables the optional multi-account mode: alongside the
// "shared" account (the legacy bot others DM, gated by the whitelist), each member of
// a household can pair their OWN WhatsApp as a "personal" device and talk to the tutor
// via self-chat — no whitelist, no admin management. Being a paired personal device is
// itself the authorization; only its OwnerPN is served by that device.
type WhatsAppDevice struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	JID       string    `gorm:"column:jid;size:100;uniqueIndex;not null" json:"jid"` // full whatsmeow device JID (device.ID.String())
	Role      string    `gorm:"size:20;default:'personal'" json:"role"`              // "shared" | "personal"
	OwnerPN   string    `gorm:"size:50;index" json:"owner_pn"`                       // owner's phone number (personal devices); the only authorized sender
	Label     string    `gorm:"size:100" json:"label"`                               // optional friendly label shown in the panel
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GroupMember is the pivot table for group membership.
type GroupMember struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	GroupID  uint      `gorm:"column:grupo_id;index;not null" json:"grupo_id"`
	UserID   uint      `gorm:"column:usuario_id;index;not null" json:"usuario_id"`
	Role     string    `gorm:"size:50;default:'MEMBER'" json:"role"` // ADMIN, MEMBER, NATIVE_MENTOR
	JoinedAt time.Time `json:"joined_at"`
}

// TableName pins the legacy table name.
func (GroupMember) TableName() string { return "grupo_membros" }
