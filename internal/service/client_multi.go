// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"chatic/internal/repository"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
)

// Roles of a paired WhatsApp device.
const (
	roleShared   = "shared"   // shared account (legacy): others DM it, whitelist applies
	rolePersonal = "personal" // a member's personal WhatsApp: only the owner's self-chat, no whitelist
)

// personalDevice is the runtime of a personal WhatsApp paired in multi-account mode.
type personalDevice struct {
	client  *whatsmeow.Client
	ownerPN string // owner's number (clean, without the :device suffix)
	jid     string // full device JID (device.ID.String())
	label   string
}

// deviceRole is the authoritative in-memory role of a paired device, resolved at event
// time by currentRole(). ownerPN is meaningful only for personal devices (the self-chat
// owner); for shared it is "".
type deviceRole struct {
	role    string // roleShared | rolePersonal
	ownerPN string
}

// currentRole resolves a client's role at the moment an event fires, rather than trusting
// a value captured when the handler was registered — a device can be promoted/demoted
// (shared<->personal) at runtime from the panel.
//
// FAIL-SAFE: a nil/not-logged-in client or an unregistered JID returns
// (rolePersonal, "") so the event is served as a personal device with no owner — i.e.
// nothing is served. It must NEVER default to shared: defaulting to shared would apply the
// shared whitelist semantics to an unknown device and could serve third parties.
func (s *WhatsAppService) currentRole(client *whatsmeow.Client) (role, ownerPN string) {
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return rolePersonal, ""
	}
	return s.roleFor(client.Store.ID.String())
}

// roleFor is the pure map lookup behind currentRole (no whatsmeow client needed, so it is
// unit-testable). An unregistered JID returns the fail-safe (rolePersonal, "").
func (s *WhatsAppService) roleFor(jid string) (role, ownerPN string) {
	s.roleMu.RLock()
	dr, ok := s.roleByJID[jid]
	s.roleMu.RUnlock()
	if !ok {
		return rolePersonal, ""
	}
	return dr.role, dr.ownerPN
}

// applySetShared returns a NEW role map in which jid is the sole shared device: any
// previously-shared device is demoted to personal (its owner set from numberOf). Pure —
// it encodes the "0 or 1 shared" invariant so SetSharedDevice and the unit tests share one
// source of truth.
func applySetShared(roles map[string]deviceRole, jid string, numberOf map[string]string) map[string]deviceRole {
	out := make(map[string]deviceRole, len(roles)+1)
	for k, v := range roles {
		if v.role == roleShared && k != jid {
			out[k] = deviceRole{role: rolePersonal, ownerPN: numberOf[k]}
		} else {
			out[k] = v
		}
	}
	out[jid] = deviceRole{role: roleShared, ownerPN: ""}
	return out
}

// applyUnsetShared returns a NEW role map with no shared device: any shared device is
// demoted to personal. Pure.
func applyUnsetShared(roles map[string]deviceRole, numberOf map[string]string) map[string]deviceRole {
	out := make(map[string]deviceRole, len(roles))
	for k, v := range roles {
		if v.role == roleShared {
			out[k] = deviceRole{role: rolePersonal, ownerPN: numberOf[k]}
		} else {
			out[k] = v
		}
	}
	return out
}

// countShared reports how many devices are marked shared (invariant: must be 0 or 1).
func countShared(roles map[string]deviceRole) int {
	n := 0
	for _, v := range roles {
		if v.role == roleShared {
			n++
		}
	}
	return n
}

// setRole records (or updates) a device's in-memory role.
func (s *WhatsAppService) setRole(jid, role, ownerPN string) {
	s.roleMu.Lock()
	s.roleByJID[jid] = deviceRole{role: role, ownerPN: ownerPN}
	s.roleMu.Unlock()
}

// forgetRole drops a device's in-memory role (e.g. after logout/removal).
func (s *WhatsAppService) forgetRole(jid string) {
	s.roleMu.Lock()
	delete(s.roleByJID, jid)
	s.roleMu.Unlock()
}

// seedRolesFromRepo populates the in-memory role map from the persisted DeviceRepository
// so events are classified correctly from the first message on boot.
func (s *WhatsAppService) seedRolesFromRepo() {
	if s.deviceRepo == nil {
		return
	}
	rows, err := s.deviceRepo.List()
	if err != nil {
		log.Printf("Warning: failed to seed device roles: %v", err)
		return
	}
	s.roleMu.Lock()
	for _, r := range rows {
		s.roleByJID[r.JID] = deviceRole{role: r.Role, ownerPN: r.OwnerPN}
	}
	s.roleMu.Unlock()
}

// AccountInfo describes a connected account for display in the panel.
type AccountInfo struct {
	JID       string `json:"jid"`
	Role      string `json:"role"`   // "shared" | "personal"
	Number    string `json:"number"` // connected number (the owner, for personal)
	Label     string `json:"label"`
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
}

// pickSharedDevice returns the ALREADY-PAIRED shared account's device, or nil when no
// shared account is paired (shared is optional). It prefers the device recorded with role
// "shared"; on a legacy install with no role records it falls back to the first paired
// device that is NOT marked "personal".
//
// It deliberately never returns a blank/unpaired device and never auto-promotes a device
// marked "personal": a personal device must not become the shared account just for coming
// first in the store. A nil return means "no shared yet" — the panel's pairing flow
// (AddDevice with role "shared") is how a shared account gets created.
func pickSharedDevice(container *sqlstore.Container, deviceRepo *repository.DeviceRepository) *store.Device {
	ctx := context.Background()

	sharedJID := ""
	personalJIDs := map[string]bool{}
	if deviceRepo != nil {
		if rows, err := deviceRepo.List(); err == nil {
			for _, r := range rows {
				switch r.Role {
				case roleShared:
					if sharedJID == "" {
						sharedJID = r.JID
					}
				case rolePersonal:
					personalJIDs[r.JID] = true
				}
			}
		}
	}

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		log.Printf("Warning: failed to list Whatsmeow devices: %v", err)
		return nil
	}

	// 1. Prefer the explicitly-marked shared device.
	if sharedJID != "" {
		for _, d := range devices {
			if d.ID != nil && d.ID.String() == sharedJID {
				return d
			}
		}
	}

	// 2. Legacy install with no shared record: the first paired device that is NOT marked
	//    personal becomes the shared account.
	for _, d := range devices {
		if d.ID != nil && !personalJIDs[d.ID.String()] {
			return d
		}
	}

	// 3. No eligible device: no shared account yet.
	return nil
}

// tagSharedDevice records (or updates) the "shared" role of the current shared device
// as soon as it becomes logged in, making the selection deterministic on subsequent
// startups, even with personal devices coexisting in the same store.
func (s *WhatsAppService) tagSharedDevice() {
	if s.deviceRepo == nil {
		return
	}
	c := s.GetClient()
	if c == nil || c.Store.ID == nil {
		return
	}
	jid := c.Store.ID.String()
	if err := s.deviceRepo.Upsert(jid, roleShared, "", "Shared account"); err != nil {
		log.Printf("Warning: failed to register shared device role: %v", err)
	}
	s.setRole(jid, roleShared, "")
}

// clientFor returns the correct WhatsApp client to send a message to a recipient:
// if the number is the owner of a personal device, it replies through that owner's own
// device (self-chat); otherwise it uses the shared account.
func (s *WhatsAppService) clientFor(number string) *whatsmeow.Client {
	number = strings.Split(number, ":")[0]
	s.personalMu.RLock()
	for _, pd := range s.personal {
		if pd.ownerPN == number && pd.client != nil {
			s.personalMu.RUnlock()
			return pd.client
		}
	}
	s.personalMu.RUnlock()
	return s.GetClient()
}

// makeEventHandler builds an event handler bound to a specific client. The role and owner
// are resolved dynamically at event time (currentRole) so a device promoted/demoted from
// the panel is served under its current role — the same processing body (handleEvent)
// serves both the shared account and the personal devices in multi-account mode.
func (s *WhatsAppService) makeEventHandler(client *whatsmeow.Client) func(interface{}) {
	return func(rawEvt interface{}) {
		role, ownerPN := s.currentRole(client)
		s.handleEvent(client, role, ownerPN, rawEvt)
	}
}

// StartPersonalDevices boots, at startup, all already-paired personal devices
// (recorded with role "personal"). Personal devices connect independently of the
// shared account. It is safe to call even with multi-account mode disabled: the flag
// only blocks pairing NEW accounts, not the operation of existing ones.
func (s *WhatsAppService) StartPersonalDevices() {
	if s.deviceRepo == nil {
		return
	}
	rows, err := s.deviceRepo.List()
	if err != nil {
		log.Printf("Warning: failed to list device roles: %v", err)
		return
	}
	type personalRec struct{ owner, label string }
	recByJID := make(map[string]personalRec)
	for _, r := range rows {
		if r.Role == rolePersonal {
			recByJID[r.JID] = personalRec{owner: r.OwnerPN, label: r.Label}
		}
	}
	if len(recByJID) == 0 {
		return
	}

	devices, err := s.dbContainer.GetAllDevices(context.Background())
	if err != nil {
		log.Printf("Warning: failed to get devices from store: %v", err)
		return
	}
	sharedJID := ""
	if c := s.GetClient(); c != nil && c.Store.ID != nil {
		sharedJID = c.Store.ID.String()
	}
	started := 0
	for _, dev := range devices {
		if dev.ID == nil {
			continue
		}
		jid := dev.ID.String()
		if jid == sharedJID {
			continue
		}
		rec, ok := recByJID[jid]
		if !ok {
			continue
		}
		s.startPersonalClient(dev, rec.owner, rec.label)
		started++
	}
	if started > 0 {
		log.Printf("Multi-account mode: %d personal device(s) started.", started)
	}
}

// startPersonalClient connects an already-paired personal device and registers it in the runtime.
func (s *WhatsAppService) startPersonalClient(dev *store.Device, ownerPN, label string) {
	client := whatsmeow.NewClient(dev, nil)
	jid := ""
	if dev.ID != nil {
		jid = dev.ID.String()
	}
	client.AddEventHandler(s.makeEventHandler(client))
	s.setRole(jid, rolePersonal, ownerPN)

	pd := &personalDevice{client: client, ownerPN: ownerPN, jid: jid, label: label}
	s.personalMu.Lock()
	s.personal[jid] = pd
	s.personalMu.Unlock()

	if err := client.Connect(); err != nil {
		log.Printf("Warning: failed to connect personal device (owner=%s): %v", ownerPN, err)
	}
}

// AddDevice starts pairing a NEW WhatsApp account with the chosen role. It creates a blank
// device in the store, opens the QR channel, and immediately returns a pendingID for the
// panel to follow the QR (GetPendingQR). On a successful pairing it starts serving:
//   - role "personal": the device serves only the owner's self-chat.
//   - role "shared":   the device becomes THE shared account (demoting any current shared
//     to personal, preserving the "0 or 1 shared" invariant).
//
// role defaults to "personal" for any value other than "shared".
func (s *WhatsAppService) AddDevice(label, role string) (string, error) {
	if role != roleShared {
		role = rolePersonal
	}
	dev := s.dbContainer.NewDevice()
	client := whatsmeow.NewClient(dev, nil)

	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return "", err
	}
	if err := client.Connect(); err != nil {
		return "", err
	}

	pendingID := newPendingID()
	s.setPendingQR(pendingID, "")

	go func() {
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				s.setPendingQR(pendingID, evt.Code)
			case "success":
				s.clearPendingQR(pendingID)
				s.finalizePairing(client, role, label)
				return
			default:
				// "timeout", "err-*", etc.: end the pending pairing.
				s.clearPendingQR(pendingID)
				if !client.IsLoggedIn() {
					client.Disconnect()
				}
				return
			}
		}
	}()

	return pendingID, nil
}

// AddDevicePhone pairs a NEW WhatsApp account via a phone pairing CODE (the "Link with phone
// number" flow) instead of a QR. It returns the short code to type on the phone; pairing
// completes asynchronously through the same finalize path as the QR flow. role defaults to
// "personal" for any value other than "shared".
func (s *WhatsAppService) AddDevicePhone(label, role, phone string) (string, error) {
	if role != roleShared {
		role = rolePersonal
	}
	dev := s.dbContainer.NewDevice()
	client := whatsmeow.NewClient(dev, nil)

	// Register the device once it finishes pairing and obtains its JID.
	var once sync.Once
	client.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.PairSuccess, *events.Connected:
			if client.Store.ID != nil {
				once.Do(func() { s.finalizePairing(client, role, label) })
			}
		}
	})

	if err := client.Connect(); err != nil {
		return "", err
	}
	code, err := client.PairPhone(context.Background(), phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		client.Disconnect()
		return "", err
	}
	return code, nil
}

// finalizePairing records a freshly-paired device and starts serving it. It always registers
// the device as "personal" first (uniform runtime state), then promotes to shared if
// requested — SetSharedDevice handles the demotion of any incumbent shared and the
// "0 or 1 shared" invariant. Shared by QR and by phone code both funnel through here.
func (s *WhatsAppService) finalizePairing(client *whatsmeow.Client, role, label string) {
	id := client.Store.ID
	if id == nil {
		log.Printf("Warning: pairing completed without a device ID.")
		return
	}
	ownerPN := strings.Split(id.User, ":")[0]
	jid := id.String()

	if s.deviceRepo != nil {
		if err := s.deviceRepo.Upsert(jid, rolePersonal, ownerPN, label); err != nil {
			log.Printf("Warning: failed to register new device: %v", err)
		}
	}
	client.AddEventHandler(s.makeEventHandler(client))
	s.setRole(jid, rolePersonal, ownerPN)
	pd := &personalDevice{client: client, ownerPN: ownerPN, jid: jid, label: label}
	s.personalMu.Lock()
	s.personal[jid] = pd
	s.personalMu.Unlock()

	if role == roleShared {
		if err := s.SetSharedDevice(jid); err != nil {
			log.Printf("Warning: failed to promote new device to shared: %v", err)
		} else {
			log.Printf("Multi-account: new device paired as the shared account.")
		}
	} else {
		log.Printf("Multi-account: new personal device paired (owner=%s).", maskPhone(ownerPN))
	}
}

// SetSharedDevice promotes an already-paired personal device (by JID) to the shared
// account. Any current shared device is demoted to a personal device that now serves only
// its own owner's self-chat. The whole swap is serialized (swapMu) so the "0 or 1 shared"
// invariant holds under concurrency.
func (s *WhatsAppService) SetSharedDevice(jid string) error {
	s.swapMu.Lock()
	defer s.swapMu.Unlock()

	s.personalMu.RLock()
	target := s.personal[jid]
	s.personalMu.RUnlock()
	if target == nil || target.client == nil {
		return fmt.Errorf("device is not an active personal device")
	}

	// 1. Demote the incumbent shared (if any) to personal.
	if cur := s.GetClient(); cur != nil && cur.Store != nil && cur.Store.ID != nil {
		curJID := cur.Store.ID.String()
		if curJID != jid {
			curNum := strings.Split(cur.Store.ID.User, ":")[0]
			const label = "Former shared"
			if s.deviceRepo != nil {
				_ = s.deviceRepo.Upsert(curJID, rolePersonal, curNum, label)
			}
			s.personalMu.Lock()
			s.personal[curJID] = &personalDevice{client: cur, ownerPN: curNum, jid: curJID, label: label}
			s.personalMu.Unlock()
			s.setRole(curJID, rolePersonal, curNum)
		}
	}

	// 2. Promote the target to shared.
	s.personalMu.Lock()
	delete(s.personal, jid)
	s.personalMu.Unlock()
	s.setClient(target.client)
	if s.deviceRepo != nil {
		_ = s.deviceRepo.Upsert(jid, roleShared, "", target.label)
	}
	s.setRole(jid, roleShared, "")
	return nil
}

// UnsetSharedDevice demotes the current shared account to a personal device (serving only
// its owner's self-chat), leaving NO shared account. Groups and inbound third-party DMs
// become unavailable until a shared account is set again.
func (s *WhatsAppService) UnsetSharedDevice() error {
	s.swapMu.Lock()
	defer s.swapMu.Unlock()

	cur := s.GetClient()
	if cur == nil || cur.Store == nil || cur.Store.ID == nil {
		return fmt.Errorf("no shared account is currently set")
	}
	curJID := cur.Store.ID.String()
	curNum := strings.Split(cur.Store.ID.User, ":")[0]
	const label = "Former shared"
	if s.deviceRepo != nil {
		_ = s.deviceRepo.Upsert(curJID, rolePersonal, curNum, label)
	}
	s.personalMu.Lock()
	s.personal[curJID] = &personalDevice{client: cur, ownerPN: curNum, jid: curJID, label: label}
	s.personalMu.Unlock()
	s.setRole(curJID, rolePersonal, curNum)
	s.setClient(nil)
	log.Printf("Multi-account: shared account demoted; no shared account is set now.")
	return nil
}

// RemoveDevice disconnects and removes a paired device (logout deletes the device from the
// whatsmeow store) and drops its role record. It handles both a personal device and the
// shared account (in which case the service is left with no shared account).
func (s *WhatsAppService) RemoveDevice(jid string) error {
	// Is this the shared account?
	if cur := s.GetClient(); cur != nil && cur.Store != nil && cur.Store.ID != nil && cur.Store.ID.String() == jid {
		if cur.IsLoggedIn() {
			_ = cur.Logout(context.Background())
		} else if cur.IsConnected() {
			cur.Disconnect()
		}
		s.setClient(nil)
		s.forgetRole(jid)
		if s.deviceRepo != nil {
			return s.deviceRepo.DeleteByJID(jid)
		}
		return nil
	}

	// Otherwise a personal device.
	s.personalMu.Lock()
	pd := s.personal[jid]
	delete(s.personal, jid)
	s.personalMu.Unlock()

	if pd != nil && pd.client != nil {
		if pd.client.IsLoggedIn() {
			_ = pd.client.Logout(context.Background())
		} else if pd.client.IsConnected() {
			pd.client.Disconnect()
		}
	}
	s.forgetRole(jid)
	if s.deviceRepo != nil {
		return s.deviceRepo.DeleteByJID(jid)
	}
	return nil
}

// maskPhone hides the subscriber tail of a phone number so the panel never
// exposes (or ships to the browser) a full real number. It keeps the leading
// country/area portion for recognition and masks the rest, e.g.
// "5511987654321" -> "551198****". Short/empty input returns "****".
func maskPhone(number string) string {
	digits := strings.TrimSpace(number)
	if len(digits) <= 4 {
		return "****"
	}
	keep := len(digits) - 4
	if keep > 6 {
		keep = 6
	}
	return digits[:keep] + "****"
}

// ListAccounts returns all connected accounts (shared + personal) for the panel.
// Phone numbers are masked here so a full number never leaves the server; personal
// accounts are returned in a deterministic (JID-sorted) order.
func (s *WhatsAppService) ListAccounts() []AccountInfo {
	var accounts []AccountInfo

	if c := s.GetClient(); c != nil {
		info := AccountInfo{
			Role:      roleShared,
			Label:     "Shared account",
			Connected: c.IsConnected(),
			LoggedIn:  c.IsLoggedIn(),
		}
		if c.Store.ID != nil {
			info.JID = c.Store.ID.String()
			info.Number = maskPhone(strings.Split(c.Store.ID.User, ":")[0])
		}
		accounts = append(accounts, info)
	}

	var personal []AccountInfo
	s.personalMu.RLock()
	for _, pd := range s.personal {
		info := AccountInfo{
			JID:    pd.jid,
			Role:   rolePersonal,
			Number: maskPhone(pd.ownerPN),
			Label:  pd.label,
		}
		if pd.client != nil {
			info.Connected = pd.client.IsConnected()
			info.LoggedIn = pd.client.IsLoggedIn()
		}
		personal = append(personal, info)
	}
	s.personalMu.RUnlock()

	sort.Slice(personal, func(i, j int) bool { return personal[i].JID < personal[j].JID })
	accounts = append(accounts, personal...)

	return accounts
}

// --- Pending pairing QR (multi-account mode) ---

func newPendingID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "pending"
	}
	return hex.EncodeToString(b)
}

func (s *WhatsAppService) setPendingQR(id, code string) {
	s.pendingMu.Lock()
	s.pendingQR[id] = code
	s.pendingMu.Unlock()
}

// GetPendingQR returns the current QR of a pending pairing and whether it is still active.
// When the pairing completes (or expires) the id ceases to exist (ok=false), signaling
// the panel to stop displaying the QR.
func (s *WhatsAppService) GetPendingQR(id string) (code string, ok bool) {
	s.pendingMu.RLock()
	code, ok = s.pendingQR[id]
	s.pendingMu.RUnlock()
	return code, ok
}

func (s *WhatsAppService) clearPendingQR(id string) {
	s.pendingMu.Lock()
	delete(s.pendingQR, id)
	s.pendingMu.Unlock()
}
