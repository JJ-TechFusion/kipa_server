package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
)

type AdminHandler struct {
	adminService service.AdminService
}

func NewAdminHandler(adminService service.AdminService) *AdminHandler {
	return &AdminHandler{
		adminService: adminService,
	}
}

func (h *AdminHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware, adminMiddleware *middleware.AdminMiddleware) {
	admin := router.Group("/admin")
	admin.Use(authMiddleware.RequireAuth())
	admin.Use(adminMiddleware.RequireAdmin())
	{
		// Rider Management
		riders := admin.Group("/riders")
		{
			riders.POST("/:id/verify", h.VerifyRider)
			riders.POST("/:id/suspend", h.SuspendRider)
			riders.POST("/:id/unsuspend", h.UnsuspendRider)
			riders.GET("/pending", h.GetPendingRiders)
			riders.GET("/statistics", h.GetRiderStatistics)
		}

		// User Management
		users := admin.Group("/users")
		{
			users.GET("", h.ListUsers)
			users.POST("/:id/suspend", h.SuspendUser)
			users.POST("/:id/unsuspend", h.UnsuspendUser)
			users.POST("/:id/verify-seller", h.VerifySeller)
			users.GET("/statistics", h.GetUserStatistics)
		}

		// Platform Settings
		settings := admin.Group("/settings")
		{
			settings.GET("/delivery-pricing", h.GetDeliveryPricing)
			settings.PUT("/delivery-pricing", h.UpdateDeliveryPricing)
			settings.GET("/platform-fees", h.GetPlatformFees)
			settings.PUT("/platform-fees", h.UpdatePlatformFees)
		}

		// Analytics
		analytics := admin.Group("/analytics")
		{
			analytics.GET("/revenue", h.GetPlatformRevenue)
			analytics.GET("/active-deliveries", h.GetActiveDeliveries)
			analytics.GET("/rider-performance", h.GetRiderPerformance)
		}

		// Payment Request Management
		paymentRequests := admin.Group("/payment-requests")
		{
			paymentRequests.GET("", h.ListPaymentRequests)
			paymentRequests.POST("/:id/cancel", h.CancelPaymentRequest)
			paymentRequests.GET("/expired", h.GetExpiredPaymentRequests)
		}

		// System Health
		health := admin.Group("/health")
		{
			health.GET("/database", h.CheckDatabaseHealth)
			health.GET("/redis", h.CheckRedisHealth)
			health.GET("/external-services", h.CheckExternalServices)
		}

		// Audit Logging
		auditLogs := admin.Group("/audit-logs")
		{
			auditLogs.GET("", h.ListAuditLogs)
			auditLogs.GET("/user/:id", h.GetUserAuditTrail)
		}

		// Dispute Management
		disputes := admin.Group("/disputes")
		{
			disputes.GET("", h.ListDisputes)
			disputes.GET("/:id", h.GetDispute)
			disputes.POST("/:id/resolve", h.ResolveDispute)
			disputes.POST("/:id/refund", h.ProcessRefund)
			disputes.GET("/statistics", h.GetDisputeStatistics)
		}

		// Commission Management
		commission := admin.Group("/settings/commission")
		{
			commission.GET("", h.GetCommissionSettings)
			commission.PUT("", h.UpdateCommissionSettings)
		}

		// Wallet Management
		wallets := admin.Group("/wallets")
		{
			wallets.GET("/user/:id", h.GetUserWallet)
			wallets.POST("/user/:id/adjust", h.AdjustWallet)
			wallets.GET("/user/:id/transactions", h.GetWalletTransactions)
		}
	}
}



// Rider Management Handlers

type VerifyRiderRequest struct {
	AdminID uuid.UUID `json:"admin_id" binding:"required"`
}

func (h *AdminHandler) VerifyRider(c *gin.Context) {
	riderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rider ID"})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.VerifyRider(c.Request.Context(), riderID, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rider verified successfully"})
}

type SuspendRiderRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *AdminHandler) SuspendRider(c *gin.Context) {
	riderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rider ID"})
		return
	}

	var req SuspendRiderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.SuspendRider(c.Request.Context(), riderID, req.Reason, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rider suspended successfully"})
}

func (h *AdminHandler) UnsuspendRider(c *gin.Context) {
	riderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rider ID"})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.UnsuspendRider(c.Request.Context(), riderID, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rider unsuspended successfully"})
}

func (h *AdminHandler) GetPendingRiders(c *gin.Context) {
	limit := 50
	offset := 0

	riders, err := h.adminService.GetPendingRiders(c.Request.Context(), limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"riders": riders,
		"count":  len(riders),
	})
}

func (h *AdminHandler) GetRiderStatistics(c *gin.Context) {
	stats, err := h.adminService.GetRiderStatistics(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

// User Management Handlers

func (h *AdminHandler) ListUsers(c *gin.Context) {
	limit := 50
	offset := 0

	users, err := h.adminService.ListUsers(c.Request.Context(), nil, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"count": len(users),
	})
}

type SuspendUserRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *AdminHandler) SuspendUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req SuspendUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.SuspendUser(c.Request.Context(), userID, req.Reason, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user suspended successfully"})
}

func (h *AdminHandler) UnsuspendUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.UnsuspendUser(c.Request.Context(), userID, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user unsuspended successfully"})
}

func (h *AdminHandler) VerifySeller(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.VerifySeller(c.Request.Context(), userID, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "seller verified successfully"})
}

func (h *AdminHandler) GetUserStatistics(c *gin.Context) {
	stats, err := h.adminService.GetUserStatistics(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

// Platform Settings Handlers

func (h *AdminHandler) GetDeliveryPricing(c *gin.Context) {
	pricing, err := h.adminService.GetDeliveryPricing(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, pricing)
}

type UpdateDeliveryPricingRequest struct {
	BaseFee            float64            `json:"base_fee" binding:"required,gte=0"`
	PerKMFee           float64            `json:"per_km_fee" binding:"required,gte=0"`
	VehicleMultipliers map[string]float64 `json:"vehicle_multipliers"`
}

func (h *AdminHandler) UpdateDeliveryPricing(c *gin.Context) {
	var req UpdateDeliveryPricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	pricing := &domain.DeliveryPricing{
		BaseFee:            req.BaseFee,
		PerKMFee:           req.PerKMFee,
		VehicleMultipliers: req.VehicleMultipliers,
	}

	if err := h.adminService.UpdateDeliveryPricing(c.Request.Context(), pricing, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "delivery pricing updated successfully"})
}

func (h *AdminHandler) GetPlatformFees(c *gin.Context) {
	fees, err := h.adminService.GetPlatformFees(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, fees)
}

type UpdatePlatformFeesRequest struct {
	BuyerFeeRate    float64 `json:"buyer_fee_rate" binding:"required,gte=0,lte=1"`
	SellerFeeRate   float64 `json:"seller_fee_rate" binding:"required,gte=0,lte=1"`
	RiderPayoutRate float64 `json:"rider_payout_rate" binding:"required,gte=0,lte=1"`
}

func (h *AdminHandler) UpdatePlatformFees(c *gin.Context) {
	var req UpdatePlatformFeesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	fees := &domain.PlatformFees{
		BuyerFeeRate:    req.BuyerFeeRate,
		SellerFeeRate:   req.SellerFeeRate,
		RiderPayoutRate: req.RiderPayoutRate,
	}

	if err := h.adminService.UpdatePlatformFees(c.Request.Context(), fees, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "platform fees updated successfully"})
}

// Analytics Handlers

func (h *AdminHandler) GetPlatformRevenue(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var start, end *string
	if startDate != "" {
		start = &startDate
	}
	if endDate != "" {
		end = &endDate
	}

	revenue, err := h.adminService.GetPlatformRevenue(c.Request.Context(), start, end)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, revenue)
}

// Error handling

func (h *AdminHandler) handleError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_server_error",
		"message": err.Error(),
	})
}

// Additional Analytics Handlers

func (h *AdminHandler) GetActiveDeliveries(c *gin.Context) {
	stats, err := h.adminService.GetActiveDeliveries(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (h *AdminHandler) GetRiderPerformance(c *gin.Context) {
	var riderID *uuid.UUID
	if id := c.Query("rider_id"); id != "" {
		parsedID, err := uuid.Parse(id)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rider_id"})
			return
		}
		riderID = &parsedID
	}

	performance, err := h.adminService.GetRiderPerformance(c.Request.Context(), riderID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"riders": performance,
		"count":  len(performance),
	})
}

// Payment Request Management Handlers

func (h *AdminHandler) ListPaymentRequests(c *gin.Context) {
	limit := 50
	offset := 0

	requests, err := h.adminService.ListPaymentRequests(c.Request.Context(), nil, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_requests": requests,
		"count":            len(requests),
	})
}

type CancelPaymentRequestRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *AdminHandler) CancelPaymentRequest(c *gin.Context) {
	prID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment request ID"})
		return
	}

	var req CancelPaymentRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.CancelPaymentRequest(c.Request.Context(), prID, req.Reason, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "payment request cancelled successfully"})
}

func (h *AdminHandler) GetExpiredPaymentRequests(c *gin.Context) {
	limit := 50
	offset := 0

	requests, err := h.adminService.GetExpiredPaymentRequests(c.Request.Context(), limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_requests": requests,
		"count":            len(requests),
	})
}

// System Health Handlers

func (h *AdminHandler) CheckDatabaseHealth(c *gin.Context) {
	health, err := h.adminService.CheckDatabaseHealth(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, health)
}

func (h *AdminHandler) CheckRedisHealth(c *gin.Context) {
	health, err := h.adminService.CheckRedisHealth(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, health)
}

func (h *AdminHandler) CheckExternalServices(c *gin.Context) {
	health, err := h.adminService.CheckExternalServices(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, health)
}

// Audit Log Handlers

func (h *AdminHandler) ListAuditLogs(c *gin.Context) {
	limit := 50
	offset := 0

	logs, err := h.adminService.ListAuditLogs(c.Request.Context(), nil, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"count": len(logs),
	})
}

func (h *AdminHandler) GetUserAuditTrail(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	limit := 50
	offset := 0

	logs, err := h.adminService.GetUserAuditTrail(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"count": len(logs),
	})
}

// Dispute Management Handlers

func (h *AdminHandler) ListDisputes(c *gin.Context) {
	limit := 50
	offset := 0

	disputes, err := h.adminService.ListDisputes(c.Request.Context(), nil, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"disputes": disputes,
		"count":    len(disputes),
	})
}

func (h *AdminHandler) GetDispute(c *gin.Context) {
	disputeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dispute ID"})
		return
	}

	details, err := h.adminService.GetDispute(c.Request.Context(), disputeID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, details)
}

type ResolveDisputeRequest struct {
	Outcome string `json:"outcome" binding:"required"`
	Notes   string `json:"notes" binding:"required"`
}

func (h *AdminHandler) ResolveDispute(c *gin.Context) {
	disputeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dispute ID"})
		return
	}

	var req ResolveDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)
	outcome := domain.DisputeOutcome(req.Outcome)

	if err := h.adminService.ResolveDispute(c.Request.Context(), disputeID, outcome, req.Notes, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "dispute resolved successfully"})
}

type ProcessRefundRequest struct {
	RefundType string  `json:"refund_type" binding:"required"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	Reason     string  `json:"reason" binding:"required"`
}

func (h *AdminHandler) ProcessRefund(c *gin.Context) {
	disputeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dispute ID"})
		return
	}

	var req ProcessRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.ProcessRefund(c.Request.Context(), disputeID, req.RefundType, req.Amount, req.Reason, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "refund processed successfully"})
}

func (h *AdminHandler) GetDisputeStatistics(c *gin.Context) {
	stats, err := h.adminService.GetDisputeStatistics(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

// Commission Management Handlers

func (h *AdminHandler) GetCommissionSettings(c *gin.Context) {
	settings, err := h.adminService.GetCommissionSettings(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, settings)
}

type UpdateCommissionSettingsRequest struct {
	MaxUnpaidRides int     `json:"max_unpaid_rides" binding:"required,min=1"`
	CommissionRate float64 `json:"commission_rate" binding:"required,gt=0,lte=1"`
}

func (h *AdminHandler) UpdateCommissionSettings(c *gin.Context) {
	var req UpdateCommissionSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	settings := &domain.CommissionSettings{
		MaxUnpaidRides: req.MaxUnpaidRides,
		CommissionRate: req.CommissionRate,
	}

	if err := h.adminService.UpdateCommissionSettings(c.Request.Context(), settings, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "commission settings updated successfully"})
}

// Wallet Management Handlers

func (h *AdminHandler) GetUserWallet(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	wallet, err := h.adminService.GetUserWallet(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, wallet)
}

type AdjustWalletRequest struct {
	AdjustmentType string  `json:"adjustment_type" binding:"required,oneof=credit debit"`
	Amount         float64 `json:"amount" binding:"required,gt=0"`
	Reason         string  `json:"reason" binding:"required"`
}

func (h *AdminHandler) AdjustWallet(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req AdjustWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.AdjustWallet(c.Request.Context(), userID, req.Amount, req.AdjustmentType, req.Reason, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "wallet adjusted successfully"})
}

func (h *AdminHandler) GetWalletTransactions(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	limit := 50
	offset := 0

	transactions, err := h.adminService.GetWalletTransactions(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
	})
}
