package websocket

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"kipa/internal/service"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		// In production, validate against allowed origins
		// For now, I'll allow common development origins
		allowedOrigins := []string{
			"http://localhost",
			"http://127.0.0.1",
			"https://kipa.ng",
			"https://app.kipa.ng",
			"https://api.kipa.ng",
		}
		for _, allowed := range allowedOrigins {
			if origin == allowed || (len(origin) > len(allowed) && origin[:len(allowed)+1] == allowed+":") {
				return true
			}
		}
		return false
	},
}

// Handler handles WebSocket upgrades
type Handler struct {
	hub         *Hub
	authService service.AuthService
	logger      *zap.Logger
}

// NewHandler creates a new WebSocket handler
func NewHandler(hub *Hub, authService service.AuthService, logger *zap.Logger) *Handler {
	return &Handler{
		hub:         hub,
		authService: authService,
		logger:      logger,
	}
}

// HandleWebSocket handles the WebSocket upgrade
// GET /api/v1/ws?token=JWT&role=user|rider&job_id=xxx
func (h *Handler) HandleWebSocket(c *gin.Context) {
	// Get query params
	token := c.Query("token")
	role := c.Query("role")
	jobID := c.Query("job_id")

	// Validate token
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token required"})
		return
	}

	claims, err := h.authService.ValidateAccessToken(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	// Validate role
	clientRole := ClientRole(role)
	if clientRole != RoleUser && clientRole != RoleRider {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be 'user' or 'rider'"})
		return
	}

	// Users must specify job_id to watch (can subscribe to more later)
	if clientRole == RoleUser && jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id required for users"})
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade connection", zap.Error(err))
		return
	}

	// Create client
	client := NewClient(h.hub, conn, claims.UserID, clientRole, claims.DeviceID, h.logger)

	// Register with hub
	h.hub.Register(client)

	// If user, subscribe to initial job
	if clientRole == RoleUser && jobID != "" {
		client.Subscribe(jobID)
	}

	// Start pumps
	go client.WritePump()
	go client.ReadPump()
}

// RegisterRoutes registers WebSocket routes
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/ws", h.HandleWebSocket)
}
