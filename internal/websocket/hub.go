package websocket

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
)
type Hub struct {
	clients map[uuid.UUID]map[*Client]bool
	rooms map[string]map[*Client]bool
	riders map[*Client]bool
	maxUserConns  int
	maxRiderConns int
	register   chan *Client
	unregister chan *Client
	locationHandler func(riderID uuid.UUID, jobID string, lat, lng, heading, speed float64)

	mu     sync.RWMutex
	logger *zap.Logger
}
func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		clients:       make(map[uuid.UUID]map[*Client]bool),
		rooms:         make(map[string]map[*Client]bool),
		riders:        make(map[*Client]bool),
		maxUserConns:  3,
		maxRiderConns: 1,
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		logger:        logger,
	}
}
func (h *Hub) SetLocationHandler(handler func(riderID uuid.UUID, jobID string, lat, lng, heading, speed float64)) {
	h.locationHandler = handler
}
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)
		}
	}
}

func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, exists := h.clients[client.UserID]; exists {
		maxConns := h.maxUserConns
		if client.Role == RoleRider {
			maxConns = h.maxRiderConns
		}

		if len(conns) >= maxConns {
			for oldest := range conns {
				h.removeClientLocked(oldest)
				break
			}
		}
	}
	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[*Client]bool)
	}
	h.clients[client.UserID][client] = true
	if client.Role == RoleRider {
		h.riders[client] = true
	}

	h.logger.Info("client registered",
		zap.String("client_id", client.ID),
		zap.String("user_id", client.UserID.String()),
		zap.String("role", string(client.Role)),
	)
}

func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeClientLocked(client)
}

func (h *Hub) removeClientLocked(client *Client) {
	if conns, ok := h.clients[client.UserID]; ok {
		delete(conns, client)
		if len(conns) == 0 {
			delete(h.clients, client.UserID)
		}
	}
	delete(h.riders, client)
	for _, room := range h.rooms {
		delete(room, client)
	}
	close(client.send)

	h.logger.Info("client unregistered", zap.String("client_id", client.ID))
}
func (h *Hub) Register(client *Client) {
	h.register <- client
}
func (h *Hub) SubscribeToJob(client *Client, jobID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[jobID] == nil {
		h.rooms[jobID] = make(map[*Client]bool)
	}
	h.rooms[jobID][client] = true

	h.logger.Debug("client subscribed to job",
		zap.String("client_id", client.ID),
		zap.String("job_id", jobID),
	)
}
func (h *Hub) UnsubscribeFromJob(client *Client, jobID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if room, ok := h.rooms[jobID]; ok {
		delete(room, client)
		if len(room) == 0 {
			delete(h.rooms, jobID)
		}
	}
}
func (h *Hub) HandleLocationUpdate(client *Client, msg *LocationUpdate) {
	if h.locationHandler != nil {
		go h.locationHandler(client.UserID, msg.JobID, msg.Lat, msg.Lng, msg.Heading, msg.Speed)
	}
	h.BroadcastToJob(msg.JobID, &domain.RiderLocation{
		Type:      domain.TypeRiderLocation,
		JobID:     msg.JobID,
		RiderID:   client.UserID.String(),
		Lat:       msg.Lat,
		Lng:       msg.Lng,
		Heading:   msg.Heading,
		Speed:     msg.Speed,
		Timestamp: time.Now().Unix(),
	})
}
func (h *Hub) BroadcastToJob(jobID string, msg interface{}) {
	h.mu.RLock()
	room := h.rooms[jobID]
	h.mu.RUnlock()

	for client := range room {
		client.Send(msg)
	}
}
func (h *Hub) BroadcastJobStatus(jobID, status string) {
	h.BroadcastToJob(jobID, &JobStatusUpdate{
		Type:   TypeJobStatus,
		JobID:  jobID,
		Status: status,
	})
}
func (h *Hub) BroadcastNewJob(job *NewJobAvailable) {
	h.mu.RLock()
	riders := make([]*Client, 0, len(h.riders))
	for rider := range h.riders {
		riders = append(riders, rider)
	}
	h.mu.RUnlock()

	for _, rider := range riders {
		rider.Send(job)
	}

	h.logger.Info("broadcast new job to riders",
		zap.String("job_id", job.JobID),
		zap.Int("rider_count", len(riders)),
	)
}
func (h *Hub) GetOnlineRiderCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.riders)
}
