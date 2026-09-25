package api

import (
	"fmt"
	"net/http"

	"whatsapp-bridge/internal/auth"
	"whatsapp-bridge/internal/database"
	"whatsapp-bridge/internal/webhook"
	"whatsapp-bridge/internal/whatsapp"
)

// Server is the HTTP REST API server for the WhatsApp bridge.
// It exposes endpoints for sending messages, managing webhooks,
// group operations, and other WhatsApp features.
type Server struct {
	client         *whatsapp.Client
	messageStore   *database.MessageStore
	webhookManager *webhook.Manager
	port           int
	bindHost       string

	// Web UI login: server-side sessions plus the configured credentials.
	sessions      *auth.Manager
	webUIUsername string
	webUIPassword string
	loginLimiter  *loginLimiter
}

// NewServer creates a new API server with the given dependencies.
// sessions may be nil, in which case web UI login is disabled and only the API key is accepted.
func NewServer(client *whatsapp.Client, messageStore *database.MessageStore, webhookManager *webhook.Manager, port int, bindHost string, sessions *auth.Manager, webUIUsername, webUIPassword string) *Server {
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	return &Server{
		client:         client,
		messageStore:   messageStore,
		webhookManager: webhookManager,
		port:           port,
		bindHost:       bindHost,
		sessions:       sessions,
		webUIUsername:  webUIUsername,
		webUIPassword:  webUIPassword,
		loginLimiter:   newLoginLimiter(),
	}
}

// Start launches the HTTP server in a background goroutine.
func (s *Server) Start() {
	s.registerHandlers()

	serverAddr := fmt.Sprintf("%s:%d", s.bindHost, s.port)
	fmt.Printf("Starting REST API server on %s...\n", serverAddr)

	go func() {
		if err := http.ListenAndServe(serverAddr, nil); err != nil {
			fmt.Printf("REST API server error: %v\n", err)
		}
	}()
}

// registerHandlers sets up all API routes with security middleware.
// All endpoints are protected by SecureMiddleware which enforces:
// API key authentication, rate limiting, CORS, and security headers.
func (s *Server) registerHandlers() {
	// Health check - no auth (for Docker healthcheck / load balancers)
	http.HandleFunc("/api/health", CorsMiddleware(s.handleHealth))

	// Web UI authentication. Login is public (it issues the session); the rest need a session or API key.
	http.HandleFunc("/api/auth/login", s.PublicMiddleware(s.handleAuthLogin))
	http.HandleFunc("/api/auth/logout", s.PublicMiddleware(s.handleAuthLogout))
	http.HandleFunc("/api/auth/me", s.PublicMiddleware(s.handleAuthMe))
	http.HandleFunc("/api/auth/sessions", s.SecureMiddleware(s.handleAuthSessions))
	http.HandleFunc("/api/auth/sessions/", s.SecureMiddleware(s.handleAuthSessionByID))

	// Message operations endpoints
	http.HandleFunc("/api/send", s.SecureMiddleware(s.handleSendMessage))
	http.HandleFunc("/api/messages", s.SecureMiddleware(s.handleGetMessages))
	http.HandleFunc("/api/chats", s.SecureMiddleware(s.handleGetChats))

	// Webhook management endpoints
	http.HandleFunc("/api/webhooks", s.SecureMiddleware(s.handleWebhooks))
	http.HandleFunc("/api/webhooks/", s.SecureMiddleware(s.handleWebhookByID))
	http.HandleFunc("/api/webhook-logs", s.SecureMiddleware(s.handleWebhookLogs))

	// Phase 1 features: Reactions, Edit, Delete, Group Info, Mark Read
	http.HandleFunc("/api/reaction", s.SecureMiddleware(s.handleReaction))
	http.HandleFunc("/api/edit", s.SecureMiddleware(s.handleEditMessage))
	http.HandleFunc("/api/delete", s.SecureMiddleware(s.handleDeleteMessage))
	http.HandleFunc("/api/group/", s.SecureMiddleware(s.handleGetGroupInfo))
	http.HandleFunc("/api/read", s.SecureMiddleware(s.handleMarkRead))

	// Phase 2: Group Management
	http.HandleFunc("/api/group/create", s.SecureMiddleware(s.handleCreateGroup))
	http.HandleFunc("/api/group/add-members", s.SecureMiddleware(s.handleAddGroupMembers))
	http.HandleFunc("/api/group/remove-members", s.SecureMiddleware(s.handleRemoveGroupMembers))
	http.HandleFunc("/api/group/promote", s.SecureMiddleware(s.handlePromoteAdmin))
	http.HandleFunc("/api/group/demote", s.SecureMiddleware(s.handleDemoteAdmin))
	http.HandleFunc("/api/group/leave", s.SecureMiddleware(s.handleLeaveGroup))
	http.HandleFunc("/api/group/update", s.SecureMiddleware(s.handleUpdateGroup))

	// Phase 3: Polls
	http.HandleFunc("/api/poll/create", s.SecureMiddleware(s.handleCreatePoll))

	// Phase 4: History Sync
	http.HandleFunc("/api/history/request", s.SecureMiddleware(s.handleRequestHistory))

	// Phase 5: Advanced Features
	http.HandleFunc("/api/presence/set", s.SecureMiddleware(s.handleSetPresence))
	http.HandleFunc("/api/presence/subscribe", s.SecureMiddleware(s.handleSubscribePresence))
	http.HandleFunc("/api/profile-picture", s.SecureMiddleware(s.handleGetProfilePicture))
	http.HandleFunc("/api/blocklist", s.SecureMiddleware(s.handleGetBlocklist))
	http.HandleFunc("/api/blocklist/update", s.SecureMiddleware(s.handleUpdateBlocklist))
	http.HandleFunc("/api/newsletter/follow", s.SecureMiddleware(s.handleFollowNewsletter))
	http.HandleFunc("/api/newsletter/unfollow", s.SecureMiddleware(s.handleUnfollowNewsletter))
	http.HandleFunc("/api/newsletter/create", s.SecureMiddleware(s.handleCreateNewsletter))

	// Phase 6: Chat Features
	http.HandleFunc("/api/typing", s.SecureMiddleware(s.handleSendTyping))
	http.HandleFunc("/api/set-about", s.SecureMiddleware(s.handleSetAbout))
	http.HandleFunc("/api/disappearing", s.SecureMiddleware(s.handleSetDisappearingTimer))
	http.HandleFunc("/api/privacy", s.SecureMiddleware(s.handleGetPrivacySettings))
	http.HandleFunc("/api/pin", s.SecureMiddleware(s.handlePinChat))
	http.HandleFunc("/api/mute", s.SecureMiddleware(s.handleMuteChat))
	http.HandleFunc("/api/archive", s.SecureMiddleware(s.handleArchiveChat))

	// Phase 7: Phone Number Pairing
	http.HandleFunc("/api/pair", s.SecureMiddleware(s.handlePairPhone))
	http.HandleFunc("/api/pairing", s.SecureMiddleware(s.handlePairingStatus))
	http.HandleFunc("/api/connection", s.SecureMiddleware(s.handleConnectionStatus))

	// Connection management
	http.HandleFunc("/api/reconnect", s.SecureMiddleware(s.handleReconnect))

	// Media download
	http.HandleFunc("/api/download", s.SecureMiddleware(s.handleDownload))

	// Sync status monitoring
	http.HandleFunc("/api/sync-status", s.SecureMiddleware(s.handleSyncStatus))
}
