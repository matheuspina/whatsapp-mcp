package whatsapp

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"whatsapp-bridge/internal/config"
	"whatsapp-bridge/internal/database"
)

// InstanceManager manages a pool of WhatsApp clients connected via whatsmeow.
// Each instance corresponds to a paired device/phone number within the organization.
type InstanceManager struct {
	mu           sync.RWMutex
	container    *sqlstore.Container
	clients      map[string]*Client // key: phone_jid or temp_id
	logger       waLog.Logger
	cfg          *config.Config
	messageStore *database.MessageStore
	defaultClient *Client

	eventHandlers []func(client *Client, evt interface{})
}

// NewInstanceManager initializes the multi-device container and loads all existing paired sessions.
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
		container:     container,
		clients:       make(map[string]*Client),
		logger:        logger,
		cfg:           cfg,
		messageStore:  messageStore,
		eventHandlers: make([]func(client *Client, evt interface{}), 0),
	}

	// Load all existing device stores
	devices, err := container.GetAllDevices(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load devices: %v", err)
	}

	if len(devices) == 0 {
		// Create initial device for backwards compatibility
		deviceStore := container.NewDevice()
		client, err := NewClientForDevice(deviceStore, logger, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create default client: %v", err)
		}
		mgr.clients["default"] = client
		mgr.defaultClient = client
		logger.Infof("[InstanceManager] Created initial default WhatsApp device")
	} else {
		for i, dev := range devices {
			client, err := NewClientForDevice(dev, logger, cfg)
			if err != nil {
				logger.Warnf("[InstanceManager] Failed to create client for device %d: %v", i, err)
				continue
			}

			key := fmt.Sprintf("device_%d", i)
			if dev.ID != nil {
				key = dev.ID.ToNonAD().String()
				// Sync to instances table in database
				if messageStore != nil {
					_, _ = messageStore.UpsertInstance(key, nil, "", "disconnected")
				}
			}

			mgr.clients[key] = client
			if mgr.defaultClient == nil {
				mgr.defaultClient = client
			}
			logger.Infof("[InstanceManager] Loaded existing device session: %s", key)
		}
	}

	return mgr, nil
}

// AddGlobalEventHandler adds an event listener to all current and future clients.
func (m *InstanceManager) AddGlobalEventHandler(handler func(client *Client, evt interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventHandlers = append(m.eventHandlers, handler)
	for _, client := range m.clients {
		c := client
		c.AddEventHandler(func(evt interface{}) {
			handler(c, evt)
		})
	}
}

// GetDefaultClient returns the primary client for single-account backwards compatibility.
func (m *InstanceManager) GetDefaultClient() *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultClient
}

// GetClient retrieves a client by phone JID or temp ID.
func (m *InstanceManager) GetClient(phoneJID string) (*Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if client, ok := m.clients[phoneJID]; ok {
		return client, nil
	}

	// Try resolving without server suffix
	for k, c := range m.clients {
		if k == phoneJID || (c.Store.ID != nil && c.Store.ID.ToNonAD().String() == phoneJID) {
			return c, nil
		}
	}

	return nil, fmt.Errorf("instance not found for JID %s", phoneJID)
}

// ListClients returns all active WhatsApp clients.
func (m *InstanceManager) ListClients() []*Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		list = append(list, c)
	}
	return list
}

// CreateNewInstance creates a new device session and returns the client and its QR code stream channel.
func (m *InstanceManager) CreateNewInstance(alias string, employeeID *int) (*Client, string, <-chan whatsmeow.QRChannelItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newDevice := m.container.NewDevice()
	client, err := NewClientForDevice(newDevice, m.logger, m.cfg)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create client for new instance: %w", err)
	}

	tempKey := fmt.Sprintf("inst_%d", time.Now().UnixNano())
	m.clients[tempKey] = client

	// Attach global event handlers to the new client
	for _, handler := range m.eventHandlers {
		h := handler
		c := client
		c.AddEventHandler(func(evt interface{}) {
			h(c, evt)
		})
	}

	// Listen for PairSuccess to migrate tempKey to real phone JID
	c := client
	client.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.PairSuccess:
			if c.Store.ID != nil {
				realJID := c.Store.ID.ToNonAD().String()
				m.mu.Lock()
				delete(m.clients, tempKey)
				m.clients[realJID] = c
				m.mu.Unlock()

				m.logger.Infof("[InstanceManager] Instance successfully paired: %s (alias: %s)", realJID, alias)
				if m.messageStore != nil {
					now := time.Now()
					_, _ = m.messageStore.UpsertInstance(realJID, employeeID, alias, "connected")
					_ = m.messageStore.UpdateInstanceStatus(realJID, "connected", &now, &now)
				}
			}
		}
	})

	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to get QR channel: %w", err)
	}

	if err := client.Client.Connect(); err != nil {
		return nil, "", nil, fmt.Errorf("failed to connect client: %w", err)
	}

	return client, tempKey, qrChan, nil
}

// DisconnectInstance disconnects an instance from WhatsApp servers.
func (m *InstanceManager) DisconnectInstance(phoneJID string) error {
	client, err := m.GetClient(phoneJID)
	if err != nil {
		return err
	}

	client.Disconnect()
	if m.messageStore != nil {
		now := time.Now()
		_ = m.messageStore.UpdateInstanceStatus(phoneJID, "disconnected", nil, &now)
	}
	return nil
}

// ReconnectInstance reconnects an existing instance.
func (m *InstanceManager) ReconnectInstance(phoneJID string) error {
	client, err := m.GetClient(phoneJID)
	if err != nil {
		return err
	}

	if client.IsConnected() {
		return nil
	}

	return client.Connect()
}

// DeleteInstance disconnects client and deletes device store from database.
func (m *InstanceManager) DeleteInstance(phoneJID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.clients[phoneJID]
	if !ok {
		return fmt.Errorf("instance %s not found", phoneJID)
	}

	client.Disconnect()
	if client.Store != nil {
		_ = client.Store.Delete(context.Background())
	}

	delete(m.clients, phoneJID)
	if m.defaultClient == client {
		m.defaultClient = nil
		for _, c := range m.clients {
			m.defaultClient = c
			break
		}
	}

	if m.messageStore != nil {
		_ = m.messageStore.DeleteInstance(phoneJID)
	}

	return nil
}
