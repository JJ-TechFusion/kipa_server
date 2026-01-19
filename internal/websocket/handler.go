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
type Handler struct {
	hub         *Hub
	authService service.AuthService
	logger      *zap.Logger
}
func NewHandler(hub *Hub, authService service.AuthService, logger *zap.Logger) *Handler {
	return &Handler{
		hub:         hub,
		authService: authService,
		logger:      logger,
	}
}
func (h *Handler) HandleWebSocket(c *gin.Context) {
	token := c.Query("token")
	role := c.Query("role")
	jobID := c.Query("job_id")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token required"})
		return
	}

	claims, err := h.authService.ValidateAccessToken(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	clientRole := ClientRole(role)
	if clientRole != RoleUser && clientRole != RoleRider {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be 'user' or 'rider'"})
		return
	}
	if clientRole == RoleUser && jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id required for users"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade connection", zap.Error(err))
		return
	}
	client := NewClient(h.hub, conn, claims.UserID, clientRole, claims.DeviceID, h.logger)
	h.hub.Register(client)
	if clientRole == RoleUser && jobID != "" {
		client.Subscribe(jobID)
	}
	go client.WritePump()
	go client.ReadPump()
}
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/ws", h.HandleWebSocket)
}
