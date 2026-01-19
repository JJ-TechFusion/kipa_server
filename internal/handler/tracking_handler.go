package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/repository"
	"kipa/internal/repository/postgres"
	"kipa/internal/util"
)
type TrackingHandler struct {
	deliveryJobRepo     repository.DeliveryJobRepository
	locationHistoryRepo *postgres.LocationHistoryRepo
	errandRepo          repository.ErrandRepository
}
func NewTrackingHandler(
	deliveryJobRepo repository.DeliveryJobRepository,
	locationHistoryRepo *postgres.LocationHistoryRepo,
	errandRepo repository.ErrandRepository,
) *TrackingHandler {
	return &TrackingHandler{
		deliveryJobRepo:     deliveryJobRepo,
		locationHistoryRepo: locationHistoryRepo,
		errandRepo:          errandRepo,
	}
}
func (h *TrackingHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	tracking := router.Group("/tracking")
	tracking.Use(authMiddleware.RequireAuth())
	{
		tracking.GET("/jobs/:id/location", h.GetJobLocation)
		tracking.GET("/jobs/:id/location/history", h.GetJobLocationHistory)
		tracking.GET("/errands/:id/location", h.GetErrandLocation)
	}
}
func (h *TrackingHandler) GetJobLocation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid job ID")
		return
	}
	job, err := h.deliveryJobRepo.GetByID(c.Request.Context(), jobID)
	if err != nil {
		if errors.Is(err, domain.ErrDeliveryJobNotFound) {
			util.Error(c, http.StatusNotFound, "NOT_FOUND", "Delivery job not found")
			return
		}
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get job")
		return
	}
	if !job.Status.IsActive() {
		util.Error(c, http.StatusBadRequest, "JOB_NOT_ACTIVE", "Job is not currently active")
		return
	}

	if job.RiderID == nil {
		util.Error(c, http.StatusBadRequest, "NO_RIDER", "No rider assigned yet")
		return
	}
	location, err := h.locationHistoryRepo.GetLatestByJobID(c.Request.Context(), jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			util.Error(c, http.StatusNotFound, "NO_LOCATION", "No location data available yet")
			return
		}
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get location")
		return
	}

	util.Success(c, http.StatusOK, LocationResponse{
		JobID:     jobID.String(),
		RiderID:   job.RiderID.String(),
		Lat:       location.Lat,
		Lng:       location.Lng,
		Heading:   location.Heading,
		Speed:     location.Speed,
		ETA:       location.ETAMinutes,
		UpdatedAt: location.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
	_ = userID
}
func (h *TrackingHandler) GetJobLocationHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid job ID")
		return
	}
	job, err := h.deliveryJobRepo.GetByID(c.Request.Context(), jobID)
	if err != nil {
		if errors.Is(err, domain.ErrDeliveryJobNotFound) {
			util.Error(c, http.StatusNotFound, "NOT_FOUND", "Delivery job not found")
			return
		}
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get job")
		return
	}
	history, err := h.locationHistoryRepo.GetByJobID(c.Request.Context(), jobID, 100)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get location history")
		return
	}

	points := make([]LocationPoint, len(history))
	for i, loc := range history {
		points[i] = LocationPoint{
			Lat:       loc.Lat,
			Lng:       loc.Lng,
			Heading:   loc.Heading,
			Speed:     loc.Speed,
			Timestamp: loc.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	util.Success(c, http.StatusOK, gin.H{
		"job_id": jobID.String(),
		"points": points,
		"count":  len(points),
	})
	_ = userID
	_ = job
}
func (h *TrackingHandler) GetErrandLocation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	errandID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid errand ID")
		return
	}
	errand, err := h.errandRepo.GetByID(c.Request.Context(), errandID)
	if err != nil {
		if errors.Is(err, domain.ErrErrandNotFound) {
			util.Error(c, http.StatusNotFound, "NOT_FOUND", "Errand not found")
			return
		}
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get errand")
		return
	}
	if errand.UserID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this errand")
		return
	}
	if !errand.IsActive() {
		util.Error(c, http.StatusBadRequest, "ERRAND_NOT_ACTIVE", "Errand is not currently active")
		return
	}

	if errand.DeliveryJobID == nil {
		util.Error(c, http.StatusBadRequest, "NO_JOB", "No delivery job created yet")
		return
	}
	location, err := h.locationHistoryRepo.GetLatestByJobID(c.Request.Context(), *errand.DeliveryJobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			util.Error(c, http.StatusNotFound, "NO_LOCATION", "No location data available yet")
			return
		}
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get location")
		return
	}

	util.Success(c, http.StatusOK, LocationResponse{
		JobID:     errand.DeliveryJobID.String(),
		RiderID:   errand.RiderID.String(),
		Lat:       location.Lat,
		Lng:       location.Lng,
		Heading:   location.Heading,
		Speed:     location.Speed,
		ETA:       location.ETAMinutes,
		UpdatedAt: location.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

type LocationResponse struct {
	JobID     string  `json:"job_id"`
	RiderID   string  `json:"rider_id"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Heading   float64 `json:"heading"`
	Speed     float64 `json:"speed"`
	ETA       *int    `json:"eta_minutes,omitempty"`
	UpdatedAt string  `json:"updated_at"`
}

type LocationPoint struct {
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Heading   float64 `json:"heading"`
	Speed     float64 `json:"speed"`
	Timestamp string  `json:"timestamp"`
}
