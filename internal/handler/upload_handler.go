package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
	"kipa/internal/util"
)

type UploadHandler struct {
	uploadService service.UploadService
}

func NewUploadHandler(uploadService service.UploadService) *UploadHandler {
	return &UploadHandler{uploadService: uploadService}
}

func (h *UploadHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	uploads := router.Group("/uploads")
	uploads.Use(authMiddleware.RequireAuth())
	{
		uploads.POST("/profile-picture", h.UploadProfilePicture)
		uploads.POST("/item-image", h.UploadItemImage)
		uploads.POST("/rider/:type", h.UploadRiderDocument)
		uploads.POST("/dispute-evidence", h.UploadDisputeEvidence)
		uploads.POST("/shipment-receipt", h.UploadShipmentReceipt)
	}
}

func (h *UploadHandler) UploadProfilePicture(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		util.Error(c, http.StatusBadRequest, "FILE_REQUIRED", "File is required")
		return
	}

	url, err := h.uploadService.UploadProfilePicture(c.Request.Context(), userID, file)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"url": url,
	})
}

func (h *UploadHandler) UploadItemImage(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		util.Error(c, http.StatusBadRequest, "FILE_REQUIRED", "File is required")
		return
	}

	url, err := h.uploadService.UploadItemImage(c.Request.Context(), userID, file)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"url": url,
	})
}

func (h *UploadHandler) UploadRiderDocument(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	docType := c.Param("type")
	if docType == "" {
		util.Error(c, http.StatusBadRequest, "INVALID_TYPE", "Document type is required")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		util.Error(c, http.StatusBadRequest, "FILE_REQUIRED", "File is required")
		return
	}

	url, err := h.uploadService.UploadRiderDocument(c.Request.Context(), userID, docType, file)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"url":  url,
		"type": docType,
	})
}

func (h *UploadHandler) UploadDisputeEvidence(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		util.Error(c, http.StatusBadRequest, "FILE_REQUIRED", "File is required")
		return
	}

	url, err := h.uploadService.UploadDisputeEvidence(c.Request.Context(), userID, file)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"url": url,
	})
}

func (h *UploadHandler) UploadShipmentReceipt(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		util.Error(c, http.StatusBadRequest, "FILE_REQUIRED", "File is required")
		return
	}

	url, err := h.uploadService.UploadShipmentReceipt(c.Request.Context(), userID, file)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"url": url,
	})
}

func (h *UploadHandler) handleError(c *gin.Context, err error) {
	errMsg := err.Error()
	switch {
	case contains(errMsg, "file type not allowed"):
		util.Error(c, http.StatusBadRequest, "INVALID_FILE_TYPE", "Only JPEG, PNG, WebP, and GIF images are allowed")
	case contains(errMsg, "file size exceeds"):
		util.Error(c, http.StatusBadRequest, "FILE_TOO_LARGE", errMsg)
	case contains(errMsg, "invalid document type"):
		util.Error(c, http.StatusBadRequest, "INVALID_DOC_TYPE", "Valid types: license, nin, face, vehicle")
	case contains(errMsg, "not configured"):
		util.Error(c, http.StatusServiceUnavailable, "UPLOAD_UNAVAILABLE", "File upload is not available")
	default:
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to upload file")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
