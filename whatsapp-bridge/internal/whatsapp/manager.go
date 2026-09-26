package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"whatsapp-bridge/internal/config"
	"whatsapp-bridge/internal/database"
	localTypes "whatsapp-bridge/internal/types"
)

var (
	// ErrInstanceNotFound is returned when a reference matches no instance.
	ErrInstanceNotFound = errors.New("instance not found")
	// ErrTooManyPairings is returned when the limit of simultaneous QR pairings is reached.
	ErrTooManyPairings = errors.New("too many pairings in progress")
)

// PairingStatus values returned by PairingQR.
const (
	PairingPending = "pending"
	PairingPaired  = "paired"
	PairingExpired = "expired"
)

// clientEntry is a client the manager owns. cancel stops the per-client goroutines started by hooks.
type clientEntry struct {
	client *Client
	ctx    context.Context
	cancel context.CancelFunc
	// manual is true after an operator disconnected the number on purpose: the connection
	// watchdog must leave it alone.
	manual bool
}

func newEntry(c *Client) *clientEntry {
	ctx, cancel := context.WithCancel(context.Background())
	return &clientEntry{client: c, ctx: ctx, cancel: cancel}
}

// pendingPairing is a QR pairing in progress. The instance row exists (status "pairing") but the
// number has no JID until the QR code is scanned.
type pendingPairing struct {
	id       int
	client   *Client
	qr       string
	qrExpiry time.Time
	status   string
}

// InstanceManager owns the pool of WhatsApp clients, one per monitored number.
//
// Every client shares one event pipeline: the manager records connection state in the instances
// table and then hands the event, together with the client it belongs to, to the registered
// handlers. Per-client goroutines (presence, circuit breaker) are started through client hooks.
type InstanceManager struct {
	mu           sync.RWMutex
	container    *sqlstore.Container
	clients      map[string]*clientEntry // key: JID without device suffix; "" for the unpaired default device
	pending      map[int]*pendingPairing // key: instances.id
	logger       waLog.Logger
	cfg          *config.Config
	messageStore *database.MessageStore

	handlers []func(client *Client, evt interface{})
	hooks    []func(ctx context.Context, client *Client)
}

// NewInstanceManager initializes the multi-device container and loads every paired session.
// When no session exists it creates an unpaired default device so the legacy pairing flow
// (terminal QR, pairing code) keeps working.
func NewInstanceManager(logger waLog.Logger, cfg *config.Config, messageStore *database.MessageStore) (*InstanceManager, error) {
	dbLog := waLog.Stdout("Database", "INFO", true)

	if err := os.MkdirAll("store", 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %v", err)
	}

	// Configure HistorySyncConfig before creating devices
	store.DeviceProps.HistorySyncConfig = &waProto.DeviceProps_HistorySyncConfig{
		FullSyncDaysLimit:              proto.Uint32(cfg.HistorySyncDaysLimit),
		FullSyncSizeMbLimit:            proto.Uint32(cfg.HistorySyncSizeMB),
		StorageQuotaMb:                 proto.Uint32(cfg.StorageQuotaMB),
		InlineInitialPayloadInE2EeMsg:  proto.Bool(true),
		SupportCallLogHistory:          proto.Bool(false),
		SupportBotUserAgentChatHistory: proto.Bool(true),
		SupportCagReactionsAndPolls:    proto.Bool(true),
		SupportGroupHistory:            proto.Bool(true),
	}

	container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to whatsapp session database: %v", err)
	}

	mgr := &InstanceManager{
		container:    container,
		clients:      make(map[string]*clientEntry),
		pending:      make(map[int]*pendingPairing),
		logger:       logger,
		cfg:          cfg,
		messageStore: messageStore,
	}

	// A pairing row left by a previous run can never finish: its QR session died with the process.
	if messageStore != nil {
		if err := messageStore.PurgeStalePendingInstances(); err != nil {
			logger.Warnf("[InstanceManager] Failed to clear stale pairing rows: %v", err)
		}
	}

	devices, err := container.GetAllDevices(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load devices: %v", err)
	}

	paired := 0
	for _, dev := range devices {
		if dev.ID == nil {
			continue
		}
		jid := dev.ID.ToNonAD().String()
		opts := ClientOptions{}
		if paired > 0 {
			// The first number keeps the configured warm-up state file (single-number installs
			// already have one); every other number gets its own.
			opts.AntibanStatePath = antibanPathFor(jid)
		}
		client, err := NewClientForDeviceWithOptions(dev, logger, cfg, opts)
		if err != nil {
			logger.Warnf("[InstanceManager] Failed to create client for %s: %v", jid, err)
			continue
		}
		paired++
		mgr.attach(client)
		mgr.clients[jid] = newEntry(client)
		if messageStore != nil {
			if _, err := messageStore.RegisterInstanceJID(jid, cfg.InstanceAllowSendDefault); err != nil {
				logger.Warnf("[InstanceManager] Failed to register instance %s: %v", jid, err)
			}
		}
		logger.Infof("[InstanceManager] Loaded existing device session: %s", jid)
	}

	if paired == 0 {
		client, err := NewClientForDevice(container.NewDevice(), logger, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create default client: %v", err)
		}
		mgr.attach(client)
		mgr.clients[""] = newEntry(client)
		logger.Infof("[InstanceManager] No paired device found: created an unpaired default device")
	}

	mgr.attributeLegacyMessages()
	return mgr, nil
}

// antibanPathFor derives a per-number warm-up state file next to the configured one.
func antibanPathFor(jid string) string {
	user := strings.SplitN(jid, "@", 2)[0]
	return fmt.Sprintf("store/antiban_warmup_%s.json", user)
}

// attributeLegacyMessages assigns messages captured before instances existed to the single
// paired number, when that is unambiguous.
func (m *InstanceManager) attributeLegacyMessages() {
	if m.messageStore == nil {
		return
	}
	var jids []string
	for key, e := range m.clients {
		if key != "" && e.client.Store.ID != nil {
			jids = append(jids, key)
		}
	}
	legacy, err := m.messageStore.CountUnattributedMessages()
	if err != nil || legacy == 0 {
		return
	}
	if len(jids) != 1 {
		m.logger.Warnf("[InstanceManager] %d messages have no instance and %d numbers are paired: they stay unattributed", legacy, len(jids))
		return
	}
	moved, err := m.messageStore.BackfillLegacyInstance(jids[0])
	if err != nil {
		m.logger.Warnf("[InstanceManager] Failed to attribute %d earlier messages to %s: %v", legacy, jids[0], err)
		return
	}
	m.logger.Infof("[InstanceManager] Attributed %d earlier messages to %s", moved, jids[0])
}

// attach installs the manager's single event handler on a client.
func (m *InstanceManager) attach(c *Client) {
	c.AddEventHandler(func(evt interface{}) {
		m.onEvent(c, evt)
	})
}

// AddGlobalEventHandler registers a handler that receives every event of every client, present
// and future, together with the client the event belongs to.
func (m *InstanceManager) AddGlobalEventHandler(handler func(client *Client, evt interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
}

// AddClientHook registers a function run for each client that becomes active: those loaded at
// startup and those paired later. The context is canceled when the client is removed, so
// goroutines started by the hook can stop with it.
func (m *InstanceManager) AddClientHook(hook func(ctx context.Context, client *Client)) {
	m.mu.Lock()
	m.hooks = append(m.hooks, hook)
	entries := make([]*clientEntry, 0, len(m.clients))
	for _, e := range m.clients {
		entries = append(entries, e)
	}
	m.mu.Unlock()

	for _, e := range entries {
		hook(e.ctx, e.client)
	}
}

// onEvent records connection state, then forwards the event to the registered handlers.
func (m *InstanceManager) onEvent(c *Client, evt interface{}) {
	switch evt.(type) {
	case *events.PairSuccess:
		m.handlePairSuccess(c)
	case *events.Connected:
		if jid := c.InstanceJID(); jid != "" && m.messageStore != nil {
			if err := m.messageStore.SetInstanceStatus(jid, database.InstanceStatusConnected, true); err != nil {
				m.logger.Warnf("[InstanceManager] Failed to record connected state for %s: %v", jid, err)
			}
		}
	case *events.Disconnected:
		if jid := c.InstanceJID(); jid != "" && m.messageStore != nil {
			_ = m.messageStore.SetInstanceStatus(jid, database.InstanceStatusDisconnected, false)
		}
	case *events.LoggedOut:
		m.handleLoggedOut(c)
	}

	m.mu.RLock()
	handlers := append([]func(*Client, interface{}){}, m.handlers...)
	m.mu.RUnlock()
	for _, h := range handlers {
		h(c, evt)
	}
}

// handlePairSuccess binds a freshly paired device to its instance row and makes it active.
func (m *InstanceManager) handlePairSuccess(c *Client) {
	jid := c.InstanceJID()
	if jid == "" {
		return
	}

	m.mu.Lock()
	var pendingID int
	for id, p := range m.pending {
		if p.client == c {
			pendingID = id
			p.status = PairingPaired
			delete(m.pending, id)
			break
		}
	}
	// Drop whichever key held this client (a pending pairing has none; the default device has "").
	for key, e := range m.clients {
		if e.client == c {
			delete(m.clients, key)
		}
	}
	entry := newEntry(c)
	m.clients[jid] = entry
	hooks := append([]func(context.Context, *Client){}, m.hooks...)
	m.mu.Unlock()

	if m.messageStore != nil {
		var err error
		if pendingID != 0 {
			_, err = m.messageStore.CompleteInstancePairing(pendingID, jid)
		} else {
			_, err = m.messageStore.RegisterInstanceJID(jid, m.cfg.InstanceAllowSendDefault)
			if err == nil {
				err = m.messageStore.SetInstanceStatus(jid, database.InstanceStatusConnected, true)
			}
		}
		if err != nil {
			m.logger.Warnf("[InstanceManager] Failed to record pairing of %s: %v", jid, err)
		}
		m.attributeLegacyMessages()
	}

	m.logger.Infof("[InstanceManager] Instance paired: %s", jid)
	for _, h := range hooks {
		h(entry.ctx, entry.client)
	}
}

// handleLoggedOut records that WhatsApp revoked the session. With other numbers still active the
// dead client leaves the pool; a lone number stays so the pairing flow can start over.
func (m *InstanceManager) handleLoggedOut(c *Client) {
	m.mu.Lock()
	jid := c.InstanceJID()
	key := ""
	for k, e := range m.clients {
		if e.client == c {
			key = k
		}
	}
	if jid == "" {
		jid = key // whatsmeow clears the device on logout
	}
	if e, ok := m.clients[key]; ok && len(m.clients) > 1 {
		e.cancel()
		delete(m.clients, key)
	}
	m.mu.Unlock()

	if jid != "" && m.messageStore != nil {
		_ = m.messageStore.SetInstanceStatus(jid, database.InstanceStatusLoggedOut, false)
	}
}

// GetDefaultClient returns the client used by endpoints that name no instance: the only paired
// number, or the first one when several exist.
func (m *InstanceManager) GetDefaultClient() *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var first *Client
	firstKey := ""
	for key, e := range m.clients {
		if first == nil || key < firstKey {
			first, firstKey = e.client, key
		}
	}
	return first
}

// ListClients returns every client the manager owns, including an unpaired default device.
func (m *InstanceManager) ListClients() []*Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Client, 0, len(m.clients))
	for _, e := range m.clients {
		list = append(list, e.client)
	}
	return list
}

// ListPairedClients returns the clients that are logged in to a number.
func (m *InstanceManager) ListPairedClients() []*Client {
	var list []*Client
	for _, c := range m.ListClients() {
		if c.Store != nil && c.Store.ID != nil {
			list = append(list, c)
		}
	}
	return list
}

// PairedCount returns how many numbers are paired.
func (m *InstanceManager) PairedCount() int { return len(m.ListPairedClients()) }

// IsManuallyDisconnected reports whether an operator disconnected the client on purpose.
func (m *InstanceManager) IsManuallyDisconnected(c *Client) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.clients {
		if e.client == c {
			return e.manual
		}
	}
	return false
}

// SetManuallyDisconnected marks a client as intentionally offline (or clears the mark).
func (m *InstanceManager) SetManuallyDisconnected(c *Client, manual bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.clients {
		if e.client == c {
			e.manual = manual
		}
	}
}

// ClientByJID returns the client for a number. The JID may carry a device suffix.
func (m *InstanceManager) ClientByJID(jid string) (*Client, error) {
	norm := normalizeJID(jid)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e, ok := m.clients[norm]; ok && norm != "" {
		return e.client, nil
	}
	return nil, ErrInstanceNotFound
}

// ResolveClient finds a client from a reference: an instance id, a JID, or a bare phone number.
func (m *InstanceManager) ResolveClient(ref string) (*Client, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, ErrInstanceNotFound
	}
	if id, err := strconv.Atoi(ref); err == nil && m.messageStore != nil {
		if inst, _ := m.messageStore.GetInstance(id); inst != nil {
			if inst.PhoneJID == "" {
				m.mu.RLock()
				defer m.mu.RUnlock()
				if p, ok := m.pending[id]; ok {
					return p.client, nil
				}
				return nil, ErrInstanceNotFound
			}
			return m.ClientByJID(inst.PhoneJID)
		}
	}
	return m.ClientByJID(ref)
}

// OnlyPairedClient returns the paired client when exactly one exists. Outbound requests use it
// to pick a sender without ambiguity.
func (m *InstanceManager) OnlyPairedClient() (*Client, bool) {
	paired := m.ListPairedClients()
	if len(paired) == 1 {
		return paired[0], true
	}
	return nil, false
}

func normalizeJID(jid string) string {
	jid = strings.TrimSpace(jid)
	if jid == "" {
		return ""
	}
	if !strings.Contains(jid, "@") {
		jid += "@s.whatsapp.net"
	}
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return jid
	}
	return parsed.ToNonAD().String()
}

// ConnectAll connects every client that is not intentionally offline, each in its own goroutine.
func (m *InstanceManager) ConnectAll(onError func(c *Client, err error)) {
	for _, c := range m.ListClients() {
		if m.IsManuallyDisconnected(c) {
			continue
		}
		go func(c *Client) {
			if err := c.Connect(); err != nil && onError != nil {
				onError(c, err)
			}
		}(c)
	}
}

// CreatePairing registers a new number and starts its QR pairing. The returned instance has
// status "pairing" until the QR code is scanned; poll PairingQR for the current code.
func (m *InstanceManager) CreatePairing(alias string, employeeID *int, allowSend bool, confirm *database.CorporateConfirmation) (*localTypes.Instance, error) {
	if m.messageStore == nil {
		return nil, errors.New("message store unavailable")
	}

	m.mu.Lock()
	if len(m.pending) >= m.cfg.MaxPendingPairings {
		m.mu.Unlock()
		return nil, ErrTooManyPairings
	}
	m.mu.Unlock()

	inst, err := m.messageStore.CreatePendingInstance(alias, employeeID, allowSend, confirm)
	if err != nil {
		return nil, err
	}

	fail := func(err error) (*localTypes.Instance, error) {
		_ = m.messageStore.DeletePendingInstance(inst.ID)
		return nil, err
	}

	client, err := NewClientForDeviceWithOptions(m.container.NewDevice(), m.logger, m.cfg, ClientOptions{NoAntibanPersistence: true})
	if err != nil {
		return fail(fmt.Errorf("failed to create client for new instance: %w", err))
	}
	m.attach(client)

	// The QR channel must be requested before connecting.
	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return fail(fmt.Errorf("failed to get QR channel: %w", err))
	}

	p := &pendingPairing{id: inst.ID, client: client, status: PairingPending}
	m.mu.Lock()
	m.pending[inst.ID] = p
	m.mu.Unlock()

	if err := client.Client.Connect(); err != nil {
		m.mu.Lock()
		delete(m.pending, inst.ID)
		m.mu.Unlock()
		return fail(fmt.Errorf("failed to connect client: %w", err))
	}

	go m.readQR(p, qrChan)
	return inst, nil
}

// readQR keeps the latest QR code of a pairing and retires the pairing when it times out.
func (m *InstanceManager) readQR(p *pendingPairing, ch <-chan whatsmeow.QRChannelItem) {
	for item := range ch {
		switch item.Event {
		case "code":
			m.mu.Lock()
			p.qr = item.Code
			p.qrExpiry = time.Now().Add(item.Timeout)
			m.mu.Unlock()
		case "success":
			return
		default:
			// "timeout" or an error: nobody scanned in time.
			m.expirePairing(p)
			return
		}
	}
}

func (m *InstanceManager) expirePairing(p *pendingPairing) {
	m.mu.Lock()
	current, ok := m.pending[p.id]
	if !ok || current != p {
		m.mu.Unlock()
		return
	}
	p.status = PairingExpired
	delete(m.pending, p.id)
	m.mu.Unlock()

	p.client.Disconnect()
	if m.messageStore != nil {
		_ = m.messageStore.DeletePendingInstance(p.id)
	}
	m.logger.Infof("[InstanceManager] Pairing %d expired without a scan", p.id)
}

// PairingQR returns the pairing state and the current QR code of a pending instance.
// Once the pairing finished or expired the pending entry is gone: the caller learns that from
// the instance row (connected) or from ErrInstanceNotFound.
func (m *InstanceManager) PairingQR(id int) (status, qr string, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.pending[id]
	if !ok {
		return "", "", ErrInstanceNotFound
	}
	if time.Now().After(p.qrExpiry) {
		return p.status, "", nil
	}
	return p.status, p.qr, nil
}

// PendingCount returns the number of pairings in progress.
func (m *InstanceManager) PendingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pending)
}

// DisconnectInstance takes a number offline on purpose; the watchdog will not reconnect it.
func (m *InstanceManager) DisconnectInstance(id int) error {
	c, inst, err := m.instanceClient(id)
	if err != nil {
		return err
	}
	m.SetManuallyDisconnected(c, true)
	c.Disconnect()
	if inst.PhoneJID != "" && m.messageStore != nil {
		return m.messageStore.SetInstanceStatus(inst.PhoneJID, database.InstanceStatusDisconnected, false)
	}
	return nil
}

// ReconnectInstance brings a number back online.
func (m *InstanceManager) ReconnectInstance(id int) error {
	c, _, err := m.instanceClient(id)
	if err != nil {
		return err
	}
	m.SetManuallyDisconnected(c, false)
	if c.IsConnected() {
		return nil
	}
	return c.Connect()
}

// RemoveInstance retires a number: it disconnects, deletes the session credentials and marks the
// instance removed. Messages already captured stay in the database, attributed to the number.
func (m *InstanceManager) RemoveInstance(id int) error {
	if m.messageStore == nil {
		return errors.New("message store unavailable")
	}
	inst, err := m.messageStore.GetInstance(id)
	if err != nil {
		return err
	}
	if inst == nil {
		return ErrInstanceNotFound
	}

	if inst.PhoneJID == "" {
		m.mu.Lock()
		p, ok := m.pending[id]
		delete(m.pending, id)
		m.mu.Unlock()
		if ok {
			p.client.Disconnect()
		}
		return m.messageStore.DeletePendingInstance(id)
	}

	m.mu.Lock()
	entry, ok := m.clients[inst.PhoneJID]
	if ok {
		delete(m.clients, inst.PhoneJID)
		if entry.cancel != nil {
			entry.cancel()
		}
	}
	m.mu.Unlock()

	if ok {
		entry.client.Disconnect()
		if entry.client.Store != nil {
			if err := entry.client.Store.Delete(context.Background()); err != nil {
				m.logger.Warnf("[InstanceManager] Failed to delete session for %s: %v", inst.PhoneJID, err)
			}
		}
	}
	return m.messageStore.MarkInstanceRemoved(id)
}

func (m *InstanceManager) instanceClient(id int) (*Client, *localTypes.Instance, error) {
	if m.messageStore == nil {
		return nil, nil, errors.New("message store unavailable")
	}
	inst, err := m.messageStore.GetInstance(id)
	if err != nil {
		return nil, nil, err
	}
	if inst == nil {
		return nil, nil, ErrInstanceNotFound
	}
	if inst.PhoneJID == "" {
		return nil, nil, fmt.Errorf("instance %d is still pairing", id)
	}
	c, err := m.ClientByJID(inst.PhoneJID)
	if err != nil {
		return nil, nil, err
	}
	return c, inst, nil
}

// Shutdown stops per-client goroutines and disconnects every client.
func (m *InstanceManager) Shutdown() {
	m.mu.Lock()
	entries := make([]*clientEntry, 0, len(m.clients))
	for _, e := range m.clients {
		entries = append(entries, e)
	}
	pend := make([]*pendingPairing, 0, len(m.pending))
	for _, p := range m.pending {
		pend = append(pend, p)
	}
	m.mu.Unlock()

	for _, e := range entries {
		if e.cancel != nil {
			e.cancel()
		}
		if err := e.client.Antiban().Close(); err != nil {
			m.logger.Warnf("Antiban close error: %v", err)
		}
		e.client.Disconnect()
	}
	for _, p := range pend {
		p.client.Disconnect()
	}
}
