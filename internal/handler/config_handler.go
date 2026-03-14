package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kipa/internal/service"
)

type ConfigHandler struct {
	adminService service.AdminService
}

func NewConfigHandler(adminService service.AdminService) *ConfigHandler {
	return &ConfigHandler{adminService: adminService}
}

func (h *ConfigHandler) RegisterRoutes(router *gin.RouterGroup) {
	config := router.Group("/config")
	{
		config.GET("/faqs", h.GetFAQs)
	}
}

func (h *ConfigHandler) GetFAQs(c *gin.Context) {
	settings, err := h.adminService.GetFAQSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get FAQs"})
		return
	}

	c.JSON(http.StatusOK, settings)
}
