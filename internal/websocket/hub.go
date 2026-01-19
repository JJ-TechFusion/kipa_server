package websocket

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Hub maintains active clients and broadcasts messages
type Hub struct {
	// Registered clients by user ID
	clients map[uuid.UUID]map[*Client]bool

	// Job rooms: jobID → clients watching
	rooms map[string]map[*Client]bool

	// Online riders (for job broadcast)
	riders map[*Client]bool

	// Connection limits
	maxUserConns  int
	maxRiderConns int

	// Channels
	register   chan *Client
	unregister chan *Client

	// Location handler (set externally)
	locationHandler func(riderID uuid.UUID, jobID string, lat, lng, heading, speed float64)

	mu     sync.RWMutex
	logger *zap.Logger
}

// NewHub creates a new hub
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

// SetLocationHandler sets the callback for location updates
func (h *Hub) SetLocationHandler(handler func(riderID uuid.UUID, jobID string, lat, lng, heading, speed float64)) {
	h.locationHandler = handler
}

// Run starts the hub's main loop
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

	// Check connection limits
	if conns, exists := h.clients[client.UserID]; exists {
		maxConns := h.maxUserConns
		if client.Role == RoleRider {
			maxConns = h.maxRiderConns
		}

		if len(conns) >= maxConns {
			// Kick oldest connection
			for oldest := range conns {
				h.removeClientLocked(oldest)
				break
			}
		}
	}

	// Add to clients map
	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[*Client]bool)
	}
	h.clients[client.UserID][client] = true

	// If rider, add to riders set
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
	// Remove from user's clients
	if conns, ok := h.clients[client.UserID]; ok {
		delete(conns, client)
		if len(conns) == 0 {
			delete(h.clients, client.UserID)
		}
	}

	// Remove from riders
	delete(h.riders, client)

	// Remove from all rooms
	for _, room := range h.rooms {
		delete(room, client)
	}

	// Close send channel
	close(client.send)

	h.logger.Info("client unregistered", zap.String("client_id", client.ID))
}

// Register adds a client to the hub
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// SubscribeToJob adds client to a job's room
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

// UnsubscribeFromJob removes client from a job's room
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

// HandleLocationUpdate processes a location update from a rider
func (h *Hub) HandleLocationUpdate(client *Client, msg *LocationUpdate) {
	// Call external handler (saves to DB, calculates ETA)
	if h.locationHandler != nil {
		go h.locationHandler(client.UserID, msg.JobID, msg.Lat, msg.Lng, msg.Heading, msg.Speed)
	}

	// Broadcast to users watching this job
	h.BroadcastToJob(msg.JobID, &RiderLocation{
		Type:      TypeRiderLocation,
		JobID:     msg.JobID,
		RiderID:   client.UserID.String(),
		Lat:       msg.Lat,
		Lng:       msg.Lng,
		Heading:   msg.Heading,
		Speed:     msg.Speed,
		Timestamp: time.Now().Unix(),
	})
}

// BroadcastToJob sends a message to all clients watching a job
func (h *Hub) BroadcastToJob(jobID string, msg interface{}) {
	h.mu.RLock()
	room := h.rooms[jobID]
	h.mu.RUnlock()

	for client := range room {
		client.Send(msg)
	}
}

// BroadcastJobStatus sends job status update to watchers
func (h *Hub) BroadcastJobStatus(jobID, status string) {
	h.BroadcastToJob(jobID, &JobStatusUpdate{
		Type:   TypeJobStatus,
		JobID:  jobID,
		Status: status,
	})
}

// BroadcastNewJob sends new job to all online riders
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

// GetOnlineRiderCount returns count of online riders
func (h *Hub) GetOnlineRiderCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.riders)
}
