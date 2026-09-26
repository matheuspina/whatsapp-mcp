package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"whatsapp-bridge/internal/antiban"
	"whatsapp-bridge/internal/api"
	"whatsapp-bridge/internal/auth"
	"whatsapp-bridge/internal/config"
	"whatsapp-bridge/internal/database"
	localTypes "whatsapp-bridge/internal/types"
	"whatsapp-bridge/internal/webhook"
	"whatsapp-bridge/internal/whatsapp"
)

// activeCall tracks an ongoing call for duration/status calculation.
type activeCall struct {
	Instance  string
	ChatJID   string
	Sender    string
	Name      string
	Timestamp time.Time
	IsFromMe  bool
}

var (
	activeCalls   = make(map[string]*activeCall)
	activeCallsMu sync.Mutex
)

// callKey scopes a call id to the number that saw it: the same group call is reported to every
// monitored number taking part in it.
func callKey(c *whatsapp.Client, callID string) string {
	return c.InstanceJID() + "|" + callID
}

// formatDuration formats a duration as "M:SS".
func formatDuration(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// resolveCallJID converts LID (hidden user) JIDs to regular phone JIDs.
func resolveCallJID(client *whatsapp.Client, logger waLog.Logger, jid types.JID) types.JID {
	if jid.Server == types.HiddenUserServer {
		resolved, err := client.Store.GetAltJID(context.Background(), jid)
		if err == nil && !resolved.IsEmpty() {
			logger.Debugf("[CALL] Resolved LID %s → %s", jid, resolved)
			return resolved
		}
		logger.Warnf("[CALL] Could not resolve LID %s: %v", jid, err)
	}
	return jid
}

// isCallFromMe checks if a call originated from this device.
func isCallFromMe(client *whatsapp.Client, from types.JID, resolvedFrom types.JID, callCreator types.JID) bool {
	if client.Store.ID == nil {
		return false
	}
	ownUser := client.Store.ID.ToNonAD().User
	if from.User == ownUser || resolvedFrom.User == ownUser {
		return true
	}
	if !callCreator.IsEmpty() && callCreator.User == ownUser {
		return true
	}
	return false
}

// fireConnectionEvent broadcasts a connection state change to all configured webhooks.
func fireConnectionEvent(wm *webhook.Manager, client *whatsapp.Client, eventType, reason string) {
	jid := ""
	if client.Store.ID != nil {
		jid = client.Store.ID.String()
	}
	wm.DeliverConnectionEvent(&localTypes.ConnectionEventPayload{
		EventType:    eventType,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		JID:          jid,
		NeedsPairing: client.Store.ID == nil,
		Reason:       reason,
	})
}

func main() {
	// Set up logger
	logger := waLog.Stdout("Client", "INFO", true)
	logger.Infof("Starting WhatsApp client...")

	// Security: Require API_KEY in production
	apiKey := os.Getenv("API_KEY")
	if config.IsPlaceholder(apiKey) {
		logger.Errorf("SECURITY: API_KEY is still the example value from .env.example")
		logger.Errorf("Generate a real one with: openssl rand -hex 32")
		os.Exit(1)
	}
	if apiKey == "" {
		if os.Getenv("DISABLE_AUTH_CHECK") != "true" {
			logger.Errorf("SECURITY: API_KEY environment variable is required")
			logger.Errorf("Set API_KEY or DISABLE_AUTH_CHECK=true for development")
			os.Exit(1)
		}
		logger.Warnf("WARNING: Running without API authentication (DISABLE_AUTH_CHECK=true)")
	} else {
		logger.Infof("API authentication enabled")
	}

	// Load configuration
	cfg := config.NewConfig()
	if config.IsPlaceholder(cfg.WebUIPassword) {
		logger.Errorf("SECURITY: WEB_UI_PASSWORD is still the example value from .env.example")
		logger.Errorf("Choose a strong password, for example: openssl rand -base64 18")
		os.Exit(1)
	}

	// Initialize database
	messageStore, err := database.NewMessageStore()
	if err != nil {
		logger.Errorf("Failed to initialize message store: %v", err)
		os.Exit(1)
	}
	defer messageStore.Close()

	// Governance settings and error reporting for queued writes nobody waits for (history sync).
	messageStore.SetRequireCorporateConfirmation(cfg.RequireCorporateConfirmation)
	messageStore.SetWriteErrorHandler(func(err error) {
		logger.Warnf("Queued database write failed: %v", err)
	})

	// Initialize WhatsApp InstanceManager (manages pool of multi-device WhatsApp accounts)
	instanceManager, err := whatsapp.NewInstanceManager(logger, cfg, messageStore)
	if err != nil {
		logger.Errorf("Failed to initialize instance manager: %v", err)
		os.Exit(1)
	}
	client := instanceManager.GetDefaultClient()

	// Initialize webhook manager
	webhookManager := webhook.NewManager(messageStore, logger)
	err = webhookManager.LoadWebhookConfigs()
	if err != nil {
		logger.Errorf("Failed to load webhook configs: %v", err)
		os.Exit(1)
	}

	// Per-number background work: started for every client that becomes active and stopped when
	// the number is removed.
	instanceManager.AddClientHook(func(ctx context.Context, c *whatsapp.Client) {
		// Fire a connection webhook just before AutoReconnect gives up, so external monitors
		// get an out-of-band alert before the watchdog exits the process.
		c.SetCircuitBreakerCallback(func() {
			fireConnectionEvent(webhookManager, c, "circuit_breaker_exhausted", "30 consecutive reconnect failures")
		})
		startPresencePing(ctx, c, cfg, logger)
	})

	// Setup global event handling across all current and future instances
	instanceManager.AddGlobalEventHandler(func(c *whatsapp.Client, evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			// Process regular messages with webhook support and instance tagging
			c.HandleMessage(messageStore, webhookManager, v)

		case *events.HistorySync:
			// Process history sync events with detailed logging
			logger.Infof("[SYNC] Starting HistorySync (Type: %v, Conversations: %d)", v.Data.SyncType, len(v.Data.Conversations))
			c.HandleHistorySync(messageStore, v)
			logger.Infof("[SYNC] ✓ Completed (Type: %v, %d conversations)", v.Data.SyncType, len(v.Data.Conversations))

		case *events.Connected:
			_, _, discAt, _ := c.ConnectionState()
			c.MarkConnected()
			c.Antiban().RecordEvent(antiban.EventConnected)
			c.ApplyConnectedPresence()
			logger.Infof("✓ Instance connected: %v", c.Store.ID)
			go fireConnectionEvent(webhookManager, c, "connected", "")

			// If we were disconnected for >30s, attempt best-effort history backfill
			// for this number's recently active chats to recover messages missed during the gap.
			if !discAt.IsZero() {
				gap := time.Since(discAt)
				if gap > 30*time.Second {
					logger.Warnf("[RECONNECT] %s: gap detected, offline for %v — attempting history backfill", c.InstanceJID(), gap.Round(time.Second))
					go backfillRecentChats(c, messageStore, logger)
				}
			}

		case *events.LoggedOut:
			c.Antiban().RecordEvent(antiban.EventLoggedOut)
			logger.Warnf("✗ Device %s logged out - credentials wiped, re-pairing required (open http://localhost:8090)", c.InstanceJID())
			// MarkDisconnected so the watchdog triggers and Docker restarts the container,
			// which will display a fresh QR / pairing code. With other numbers active the manager
			// drops just this client, and the watchdog only looks at the remaining ones.
			c.MarkDisconnected()
			go fireConnectionEvent(webhookManager, c, "logged_out", "session revoked by WhatsApp server")

		case *events.PairSuccess:
			logger.Infof("✓ Phone pairing successful!")
			c.HandlePairingSuccess()
			go fireConnectionEvent(webhookManager, c, "pair_success", "")

		case *events.PairError:
			logger.Errorf("✗ Phone pairing failed: %v", v.Error)
			c.HandlePairingError(v.Error)
			errMsg := ""
			if v.Error != nil {
				errMsg = v.Error.Error()
			}
			go fireConnectionEvent(webhookManager, c, "pair_error", errMsg)

		case *events.KeepAliveTimeout:
			c.Antiban().RecordEvent(antiban.EventKeepAliveTimeout)
			logger.Warnf("⚠ KeepAlive timeout on %s (errors: %d)", c.InstanceJID(), v.ErrorCount)
			if v.ErrorCount >= 3 {
				logger.Errorf("KeepAlive: %d consecutive failures on %s, forcing disconnect+reconnect", v.ErrorCount, c.InstanceJID())
				c.Disconnect()
				go func() {
					time.Sleep(2 * time.Second)
					if err := c.Client.Connect(); err != nil {
						logger.Errorf("Reconnect after KeepAlive failure: %v", err)
					}
				}()
			}

		case *events.KeepAliveRestored:
			logger.Infof("✓ KeepAlive restored after timeout")

		case *events.StreamReplaced:
			// Another process has taken over this session (e.g. duplicate docker-compose up).
			// Two processes sharing one WhatsApp session cause split-brain. A lone number exits so
			// the container restarts; with several numbers only this one is taken offline, since
			// the others are unaffected.
			c.MarkDisconnected()
			if instanceManager.PairedCount() <= 1 {
				logger.Errorf("✗ Stream replaced — another process took this session, exiting")
				os.Exit(1)
			}
			logger.Errorf("✗ Stream replaced on %s — another process took this session; the number stays offline until reconnected", c.InstanceJID())
			instanceManager.SetManuallyDisconnected(c, true)
			c.Disconnect()

		case *events.StreamError:
			c.Antiban().RecordEvent(antiban.EventStreamError)
			logger.Errorf("✗ Stream error: %v", v.Code)

		case *events.Disconnected:
			c.MarkDisconnected()
			c.ResetPresenceState()
			c.Antiban().RecordEvent(antiban.EventDisconnected)
			logger.Warnf("⚠ %s disconnected from WhatsApp - attempting reconnect", c.InstanceJID())
			go fireConnectionEvent(webhookManager, c, "disconnected", "")

		case *events.CallOffer:
			handleCallOffer(c, messageStore, logger, v.From, v.CallCreator, v.GroupJID, v.CallID, v.Timestamp, "")

		case *events.CallOfferNotice:
			handleCallOffer(c, messageStore, logger, v.From, v.CallCreator, v.GroupJID, v.CallID, v.Timestamp, v.Media)

		case *events.CallAccept:
			resolvedJID := resolveCallJID(c, logger, v.From)
			logger.Infof("[CALL] CallAccept from %s (CallID: %s)", resolvedJID.User, v.CallID)
			activeCallsMu.Lock()
			if call, exists := activeCalls[callKey(c, v.CallID)]; exists {
				call.Timestamp = v.Timestamp
			}
			activeCallsMu.Unlock()

		case *events.CallTerminate:
			resolvedJID := resolveCallJID(c, logger, v.From)
			logger.Infof("[CALL] CallTerminate from %s (CallID: %s, Reason: %s)", resolvedJID.User, v.CallID, v.Reason)
			activeCallsMu.Lock()
			key := callKey(c, v.CallID)
			call, exists := activeCalls[key]
			if exists {
				delete(activeCalls, key)
			}
			activeCallsMu.Unlock()
			if exists {
				duration := v.Timestamp.Sub(call.Timestamp)
				var content string
				switch v.Reason {
				case "timeout", "busy":
					content = fmt.Sprintf("📞 Missed call from %s", call.Name)
				default:
					content = fmt.Sprintf("📞 Call with %s (%s)", call.Name, formatDuration(duration))
				}
				if err := messageStore.StoreMessageWithInstance("call-"+v.CallID, call.ChatJID, call.Sender, call.Name, content, call.Timestamp, call.IsFromMe, "call", "", "", "", nil, nil, nil, 0, call.Instance, false); err != nil {
					logger.Warnf("Failed to update call message: %v", err)
				}
			}

		case *events.CallReject:
			resolvedJID := resolveCallJID(c, logger, v.From)
			logger.Infof("[CALL] CallReject from %s (CallID: %s)", resolvedJID.User, v.CallID)
			activeCallsMu.Lock()
			key := callKey(c, v.CallID)
			call, exists := activeCalls[key]
			if exists {
				delete(activeCalls, key)
			}
			activeCallsMu.Unlock()
			if exists {
				content := fmt.Sprintf("📞 Missed call from %s", call.Name)
				if err := messageStore.StoreMessageWithInstance("call-"+v.CallID, call.ChatJID, call.Sender, call.Name, content, call.Timestamp, call.IsFromMe, "call", "", "", "", nil, nil, nil, 0, call.Instance, false); err != nil {
					logger.Warnf("Failed to update rejected call message: %v", err)
				}
			}
		}
	})

	// Connection watchdog. A number that stays disconnected for >3 min is reconnected; when every
	// paired number has been down that long the process exits to force a container restart (the
	// behaviour a single-number install always had). Numbers an operator took offline on purpose
	// are left alone.
	go watchConnections(instanceManager, logger)

	// Stale call cleanup: remove calls older than 5 minutes without terminate event
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-5 * time.Minute)
			activeCallsMu.Lock()
			for id, call := range activeCalls {
				if call.Timestamp.Before(cutoff) {
					delete(activeCalls, id)
					logger.Debugf("[CALL] Cleaned up stale call %s", id)
				}
			}
			activeCallsMu.Unlock()
		}
	}()

	// Start REST API server with webhook support (BEFORE connecting to avoid blocking)
	var sessions *auth.Manager
	if cfg.WebUIUsername != "" && cfg.WebUIPassword != "" {
		sessions = auth.NewManagerWithStore(cfg.WebUISessionTTL, messageStore)
		sessions.StartCleanup(time.Minute)
		defer sessions.Stop()
		logger.Infof("Web UI login enabled for user %q (session TTL %v)", cfg.WebUIUsername, cfg.WebUISessionTTL)
	} else {
		logger.Warnf("Web UI login disabled: set WEB_UI_USERNAME and WEB_UI_PASSWORD to enable it")
	}
	server := api.NewServer(client, messageStore, webhookManager, cfg.APIPort, cfg.APIBindHost, sessions, cfg.WebUIUsername, cfg.WebUIPassword)
	server.SetInstanceManager(instanceManager)
	server.Start()
	fmt.Println("✓ REST API server started on port " + fmt.Sprintf("%d", cfg.APIPort))

	// Connect all initialized WhatsApp devices in background (non-blocking so server can start)
	instanceManager.ConnectAll(func(c *whatsapp.Client, err error) {
		logger.Errorf("Failed to connect instance %s: %v", c.InstanceJID(), err)
	})

	// Retention policy: delete captured messages older than RETENTION_DAYS, once a day.
	if cfg.RetentionDays > 0 {
		go runRetention(messageStore, cfg.RetentionDays, logger)
	}

	// Create a channel to keep the main goroutine alive
	exitChan := make(chan os.Signal, 1)
	signal.Notify(exitChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("REST server is running. Press Ctrl+C to disconnect and exit.")
	fmt.Println("=" + fmt.Sprintf("%150s", ""))
	fmt.Println("Monitor sync progress:")
	// Never print the key itself: container logs are routinely copied around.
	fmt.Println("  curl -H \"X-API-Key: $API_KEY\" http://localhost:" + fmt.Sprintf("%d", cfg.APIPort) + "/api/sync-status")
	fmt.Println("=" + fmt.Sprintf("%150s", ""))

	// Periodically log sync stats
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			logger.Debugf("[STATS] %d paired, %d pairings in progress", instanceManager.PairedCount(), instanceManager.PendingCount())
		}
	}()

	// Wait for termination signal
	<-exitChan

	fmt.Println("Disconnecting all instances...")
	instanceManager.Shutdown()
}

// startPresencePing keeps a number's session active with a periodic presence ping. Controlled by
// PRESENCE_PING_ENABLED and PRESENCE_PING_INTERVAL. Default: enabled, every 20 minutes. In human
// presence mode the ping sends "unavailable" so the session stays alive without showing the
// account online. The goroutine stops with ctx, which is canceled when the number is removed.
func startPresencePing(ctx context.Context, c *whatsapp.Client, cfg *config.Config, logger waLog.Logger) {
	if !cfg.PresencePingEnabled {
		return
	}
	pingPresence := "available"
	if c.PresenceMode() == whatsapp.PresenceModeHuman {
		pingPresence = "unavailable"
	}
	go func() {
		ticker := time.NewTicker(cfg.PresencePingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			// Never override an active online window with a keepalive ping.
			if c.IsConnected() && !c.PresenceOnline() {
				if err := c.SetPresence(pingPresence); err != nil {
					logger.Debugf("Presence ping failed: %v", err)
				} else {
					logger.Debugf("Presence ping sent as %s (interval: %v)", pingPresence, cfg.PresencePingInterval)
				}
			}
		}
	}()
}

// watchConnections reconnects numbers that stayed offline and restarts the process when all of them did.
func watchConnections(mgr *whatsapp.InstanceManager, logger waLog.Logger) {
	const offlineLimit = 3 * time.Minute
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		paired := mgr.ListPairedClients()
		if len(paired) == 0 {
			// Unpaired default device: same rule as before, based on its own connection state.
			for _, c := range mgr.ListClients() {
				_, _, discAt, _ := c.ConnectionState()
				if !discAt.IsZero() && time.Since(discAt) > offlineLimit {
					logger.Errorf("WATCHDOG: disconnected for %v, exiting to force container restart", time.Since(discAt).Round(time.Second))
					os.Exit(1)
				}
			}
			continue
		}

		watched, allDown := 0, true
		for _, c := range paired {
			if mgr.IsManuallyDisconnected(c) {
				continue
			}
			watched++
			_, _, discAt, _ := c.ConnectionState()
			down := !discAt.IsZero() && time.Since(discAt) > offlineLimit
			if !down {
				allDown = false
				continue
			}
			if len(paired) > 1 {
				// Other numbers are still up, so this one gets another try instead of a restart.
				logger.Warnf("WATCHDOG: %s offline for %v, reconnecting", c.InstanceJID(), time.Since(discAt).Round(time.Second))
				go func(c *whatsapp.Client) {
					if err := c.Client.Connect(); err != nil {
						logger.Warnf("WATCHDOG: reconnect of %s failed: %v", c.InstanceJID(), err)
					}
				}(c)
			}
		}
		if watched > 0 && allDown {
			logger.Errorf("WATCHDOG: every paired number has been disconnected for over %v, exiting to force container restart", offlineLimit)
			os.Exit(1)
		}
	}
}

// backfillRecentChats asks WhatsApp for messages a number missed while it was offline.
func backfillRecentChats(c *whatsapp.Client, messageStore *database.MessageStore, logger waLog.Logger) {
	time.Sleep(5 * time.Second) // let WA session stabilise first
	instanceJID := c.InstanceJID()
	chats, err := messageStore.ListChatsForInstance(instanceJID)
	if err != nil {
		logger.Warnf("[RECONNECT] Failed to get chats for backfill: %v", err)
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	requested := 0
	for chatJID, lastMsgTime := range chats {
		if lastMsgTime.Before(cutoff) {
			continue // skip inactive chats
		}
		newest, err := messageStore.GetNewestMessageForInstance(instanceJID, chatJID)
		if err != nil || newest == nil {
			continue
		}
		if err := c.RequestChatHistory(chatJID, newest.ID, newest.IsFromMe, newest.Sender, newest.Time.UnixMilli(), 50); err != nil {
			logger.Warnf("[RECONNECT] History request failed for %s: %v", chatJID, err)
		} else {
			logger.Infof("[RECONNECT] History requested for %s (last active: %v)", chatJID, lastMsgTime.Format("15:04:05"))
			requested++
		}
		time.Sleep(500 * time.Millisecond) // avoid rate limiting
	}
	logger.Infof("[RECONNECT] %s: backfill requested for %d active chats", instanceJID, requested)
}

// handleCallOffer records an incoming or outgoing call as a message of the number that saw it.
func handleCallOffer(c *whatsapp.Client, messageStore *database.MessageStore, logger waLog.Logger, from, callCreator, groupJID types.JID, callID string, ts time.Time, media string) {
	resolvedJID := resolveCallJID(c, logger, from)
	callFrom := resolvedJID.User
	fromMe := isCallFromMe(c, from, resolvedJID, callCreator)
	var chatJID string
	var chatResolvedJID types.JID
	if !groupJID.IsEmpty() {
		chatJID = groupJID.String()
		chatResolvedJID = groupJID
	} else {
		chatJID = resolvedJID.String()
		chatResolvedJID = resolvedJID
	}
	logger.Infof("[CALL] CallOffer from %s (CallID: %s, isFromMe: %v)", callFrom, callID, fromMe)
	name := c.GetChatName(messageStore, chatResolvedJID, chatJID, nil, callFrom)

	kind := ""
	if media != "" {
		kind = media + " "
	}
	var content string
	if fromMe {
		content = fmt.Sprintf("📞 Outgoing %scall to %s", kind, name)
	} else {
		content = fmt.Sprintf("📞 Incoming %scall from %s", kind, name)
	}

	instanceJID := c.InstanceJID()
	activeCallsMu.Lock()
	activeCalls[callKey(c, callID)] = &activeCall{Instance: instanceJID, ChatJID: chatJID, Sender: callFrom, Name: name, Timestamp: ts, IsFromMe: fromMe}
	activeCallsMu.Unlock()
	if err := messageStore.StoreChatWithInstance(chatJID, name, ts, instanceJID); err != nil {
		logger.Warnf("Failed to store chat for call: %v", err)
	}
	if err := messageStore.StoreMessageWithInstance("call-"+callID, chatJID, callFrom, name, content, ts, fromMe, "call", "", "", "", nil, nil, nil, 0, instanceJID, false); err != nil {
		logger.Warnf("Failed to store call message: %v", err)
	}
}

// runRetention deletes messages older than the retention window at startup and then daily.
func runRetention(messageStore *database.MessageStore, days int, logger waLog.Logger) {
	run := func() {
		cutoff := time.Now().UTC().AddDate(0, 0, -days)
		removed, err := messageStore.PurgeOlderThan(cutoff, "retention-policy")
		if err != nil {
			logger.Warnf("[RETENTION] purge failed: %v", err)
			return
		}
		if removed > 0 {
			logger.Infof("[RETENTION] removed %d messages older than %d days", removed, days)
		}
	}
	run()
	for range time.Tick(24 * time.Hour) {
		run()
	}
}
