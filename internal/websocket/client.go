package websocket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	writeWait = 10 * time.Second
	pongWait = 60 * time.Second
	pingPeriod = 30 * time.Second
	maxMessageSize = 4096
)
type ClientRole string

const (
	RoleUser  ClientRole = "user"
	RoleRider ClientRole = "rider"
)
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	logger *zap.Logger
	ID       string
	UserID   uuid.UUID
	Role     ClientRole
	DeviceID string
	ActiveJobID *uuid.UUID
	watchedJobs map[string]bool
	mu          sync.RWMutex
}
func NewClient(hub *Hub, conn *websocket.Conn, userID uuid.UUID, role ClientRole, deviceID string, logger *zap.Logger) *Client {
	return &Client{
		hub:         hub,
		conn:        conn,
		send:        make(chan []byte, 256),
		logger:      logger,
		ID:          uuid.New().String(),
		UserID:      userID,
		Role:        role,
		DeviceID:    deviceID,
		watchedJobs: make(map[string]bool),
	}
}
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Warn("websocket read error", zap.Error(err))
			}
			break
		}

		c.handleMessage(message)
	}
}
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
func (c *Client) handleMessage(data []byte) {
	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		c.sendError("PARSE_ERROR", "Invalid message format")
		return
	}

	switch base.Type {
	case TypeLocationUpdate:
		if c.Role != RoleRider {
			c.sendError("FORBIDDEN", "Only riders can send location updates")
			return
		}
		var msg LocationUpdate
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendError("PARSE_ERROR", "Invalid location update")
			return
		}
		c.hub.HandleLocationUpdate(c, &msg)

	case TypeSubscribe:
		if c.Role != RoleUser {
			c.sendError("FORBIDDEN", "Only users can subscribe to jobs")
			return
		}
		var msg Subscribe
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendError("PARSE_ERROR", "Invalid subscribe message")
			return
		}
		c.Subscribe(msg.JobID)

	case TypeUnsubscribe:
		var msg Unsubscribe
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendError("PARSE_ERROR", "Invalid unsubscribe message")
			return
		}
		c.Unsubscribe(msg.JobID)

	case TypePing:
		c.sendPong()

	case TypeChatMessage:
		var msg ChatMessagePayload
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendError("PARSE_ERROR", "Invalid chat message")
			return
		}
		c.hub.HandleChatMessage(c, &msg)

	default:
		c.sendError("UNKNOWN_TYPE", "Unknown message type: "+base.Type)
	}
}
func (c *Client) Subscribe(jobID string) {
	c.mu.Lock()
	c.watchedJobs[jobID] = true
	c.mu.Unlock()
	c.hub.SubscribeToJob(c, jobID)
}
func (c *Client) Unsubscribe(jobID string) {
	c.mu.Lock()
	delete(c.watchedJobs, jobID)
	c.mu.Unlock()
	c.hub.UnsubscribeFromJob(c, jobID)
}
func (c *Client) GetWatchedJobs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	jobs := make([]string, 0, len(c.watchedJobs))
	for j := range c.watchedJobs {
		jobs = append(jobs, j)
	}
	return jobs
}
func (c *Client) Send(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("failed to marshal message", zap.Error(err))
		return
	}

	select {
	case c.send <- data:
	default:
		c.logger.Warn("client send buffer full", zap.String("client_id", c.ID))
	}
}

func (c *Client) sendError(code, message string) {
	c.Send(&ErrorMessage{
		Type:    TypeError,
		Code:    code,
		Message: message,
	})
}

func (c *Client) sendPong() {
	c.Send(&BaseMessage{Type: TypePong})
}
