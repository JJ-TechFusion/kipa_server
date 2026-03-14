package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/dto"
	"kipa/internal/handler/middleware"
	"kipa/internal/repository"
	"kipa/internal/service"
)

type AdminHandler struct {
	adminService             service.AdminService
	disputeResolutionService service.DisputeResolutionService
	deliveryJobRepo          repository.DeliveryJobRepository
	paymentRequestService    service.PaymentRequestService
	supportStaffService      service.SupportStaffService
}

func NewAdminHandler(
	adminService service.AdminService,
	disputeResolutionService service.DisputeResolutionService,
	deliveryJobRepo repository.DeliveryJobRepository,
	paymentRequestService service.PaymentRequestService,
	supportStaffService service.SupportStaffService,
) *AdminHandler {
	return &AdminHandler{
		adminService:             adminService,
		disputeResolutionService: disputeResolutionService,
		deliveryJobRepo:          deliveryJobRepo,
		paymentRequestService:    paymentRequestService,
		supportStaffService:      supportStaffService,
	}
}

func (h *AdminHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware, adminMiddleware *middleware.AdminMiddleware) {
	admin := router.Group("/admin")
	admin.Use(authMiddleware.RequireAuth())
	admin.Use(adminMiddleware.RequireAdmin())
	{
		riders := admin.Group("/riders")
		{
			riders.POST("/:id/verify", h.VerifyRider)
			riders.POST("/:id/reject", h.RejectRider)
			riders.POST("/:id/suspend", h.SuspendRider)
			riders.POST("/:id/unsuspend", h.UnsuspendRider)
			riders.GET("/pending", h.GetPendingRiders)
			riders.GET("/statistics", h.GetRiderStatistics)
		}
		drivers := admin.Group("/drivers")
		{
			drivers.POST("/:id/verify", h.VerifyDriver)
			drivers.POST("/:id/suspend", h.SuspendDriver)
			drivers.POST("/:id/unsuspend", h.UnsuspendDriver)
			drivers.GET("/pending", h.GetPendingDrivers)
			drivers.GET("/statistics", h.GetDriverStatistics)
		}
		admin.POST("/notifications/broadcast", h.BroadcastNotification)
		users := admin.Group("/users")
		{
			users.GET("", h.ListUsers)
			users.POST("/:id/suspend", h.SuspendUser)
			users.POST("/:id/unsuspend", h.UnsuspendUser)
			users.POST("/:id/verify-seller", h.VerifySeller)
			users.GET("/statistics", h.GetUserStatistics)
		}
		settings := admin.Group("/settings")
		{
			settings.GET("/delivery-pricing", h.GetDeliveryPricing)
			settings.PUT("/delivery-pricing", h.UpdateDeliveryPricing)
			settings.GET("/platform-fees", h.GetPlatformFees)
			settings.PUT("/platform-fees", h.UpdatePlatformFees)
		}
		analytics := admin.Group("/analytics")
		{
			analytics.GET("/revenue", h.GetPlatformRevenue)
			analytics.GET("/active-deliveries", h.GetActiveDeliveries)
			analytics.GET("/rider-performance", h.GetRiderPerformance)
			analytics.GET("/transactions", h.ListAllTransactions)
			analytics.GET("/delivery-jobs", h.ListAllDeliveryJobs)
		}
		paymentRequests := admin.Group("/payment-requests")
		{
			paymentRequests.GET("", h.ListPaymentRequests)
			paymentRequests.POST("/:id/cancel", h.CancelPaymentRequest)
			paymentRequests.GET("/expired", h.GetExpiredPaymentRequests)
			paymentRequests.POST("/:id/rebook", h.AdminRebookDelivery) // Rebook on buyer's behalf
		}
		health := admin.Group("/health")
		{
			health.GET("/database", h.CheckDatabaseHealth)
			health.GET("/redis", h.CheckRedisHealth)
			health.GET("/external-services", h.CheckExternalServices)
		}
		auditLogs := admin.Group("/audit-logs")
		{
			auditLogs.GET("", h.ListAuditLogs)
			auditLogs.GET("/user/:id", h.GetUserAuditTrail)
		}
		disputes := admin.Group("/disputes")
		{
			disputes.GET("", h.ListDisputes)
			disputes.GET("/:id", h.GetDispute)
			disputes.POST("/:id/resolve", h.ResolveDispute)
			disputes.POST("/:id/refund", h.ProcessRefund)
			disputes.GET("/statistics", h.GetDisputeStatistics)
		}
		returnClaims := admin.Group("/return-claims")
		{
			returnClaims.GET("", h.ListReturnDamageClaims)
			returnClaims.GET("/:id", h.GetReturnDamageClaim)
			returnClaims.POST("/:id/resolve", h.ResolveReturnDamageClaim)
		}
		commission := admin.Group("/settings/commission")
		{
			commission.GET("", h.GetCommissionSettings)
			commission.PUT("", h.UpdateCommissionSettings)
		}
		wallets := admin.Group("/wallets")
		{
			wallets.GET("/user/:id", h.GetUserWallet)
			wallets.POST("/user/:id/adjust", h.AdjustWallet)
			wallets.GET("/user/:id/transactions", h.GetWalletTransactions)
		}
		supportStaff := admin.Group("/support-staff")
		{
			supportStaff.POST("", h.AddSupportStaff)
			supportStaff.GET("", h.ListSupportStaff)
			supportStaff.GET("/:user_id", h.GetSupportStaff)
			supportStaff.PUT("/:user_id", h.UpdateSupportStaff)
			supportStaff.DELETE("/:user_id", h.RemoveSupportStaff)
		}
	}
}

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

type RejectRiderRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *AdminHandler) RejectRider(c *gin.Context) {
	riderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rider ID"})
		return
	}

	var req RejectRiderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.RejectRider(c.Request.Context(), riderID, req.Reason, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rider verification rejected"})
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

func (h *AdminHandler) VerifyDriver(c *gin.Context) {
	driverID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid driver ID"})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.VerifyDriver(c.Request.Context(), driverID, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "driver verified successfully"})
}

func (h *AdminHandler) SuspendDriver(c *gin.Context) {
	driverID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid driver ID"})
		return
	}

	var req SuspendRiderRequest // Reuse same request struct as it only contains 'reason'
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.SuspendDriver(c.Request.Context(), driverID, req.Reason, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "driver suspended successfully"})
}

func (h *AdminHandler) UnsuspendDriver(c *gin.Context) {
	driverID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid driver ID"})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	if err := h.adminService.UnsuspendDriver(c.Request.Context(), driverID, adminID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "driver unsuspended successfully"})
}

func (h *AdminHandler) GetPendingDrivers(c *gin.Context) {
	limit := 50
	offset := 0

	drivers, err := h.adminService.GetPendingDrivers(c.Request.Context(), limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"drivers": drivers,
		"count":   len(drivers),
	})
}

func (h *AdminHandler) GetDriverStatistics(c *gin.Context) {
	stats, err := h.adminService.GetDriverStatistics(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

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

func (h *AdminHandler) handleError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_server_error",
		"message": err.Error(),
	})
}

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
func (h *AdminHandler) AdminRebookDelivery(c *gin.Context) {
	prID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment request ID"})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	job, err := h.paymentRequestService.AdminRebookDelivery(c.Request.Context(), prID, adminID, &service.RebookDeliveryRequest{})
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "delivery rebooked successfully",
		"job_id":  job.ID.String(),
		"status":  string(job.Status),
	})
}

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
	Outcome               string   `json:"outcome" binding:"required"`
	Notes                 string   `json:"notes" binding:"required"`
	ReturnCostBuyerShare  *float64 `json:"return_cost_buyer_share" binding:"omitempty,min=0,max=100"`
	ReturnCostSellerShare *float64 `json:"return_cost_seller_share" binding:"omitempty,min=0,max=100"`
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
	serviceReq := &service.ResolveDisputeRequest{
		Outcome:               req.Outcome,
		ResolutionNotes:       req.Notes,
		ReturnCostBuyerShare:  req.ReturnCostBuyerShare,
		ReturnCostSellerShare: req.ReturnCostSellerShare,
	}

	if err := h.disputeResolutionService.ResolveDispute(c.Request.Context(), disputeID, adminID, serviceReq); err != nil {
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

type BroadcastNotificationRequest struct {
	Title string `json:"title" binding:"required"`
	Body  string `json:"body" binding:"required"`
	Topic string `json:"topic"` // e.g. "all_users", "all_riders", "all_sellers"
}

func (h *AdminHandler) BroadcastNotification(c *gin.Context) {
	adminID, exists := c.Get("user_id")
	if !exists {
		h.handleError(c, domain.ErrUnauthorized)
		return
	}

	var req BroadcastNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.adminService.SendBroadcast(c.Request.Context(), req.Title, req.Body, req.Topic, adminID.(uuid.UUID)); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Broadcast sent successfully"})
}

func (h *AdminHandler) ListReturnDamageClaims(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	claims, total, err := h.disputeResolutionService.ListPendingDamageClaims(c.Request.Context(), limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]ReturnDamageClaimResponse, 0, len(claims))
	for _, claim := range claims {
		responses = append(responses, toReturnDamageClaimResponse(claim))
	}

	c.JSON(http.StatusOK, gin.H{
		"claims": responses,
		"total":  total,
	})
}

func (h *AdminHandler) GetReturnDamageClaim(c *gin.Context) {
	claimID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid claim ID"})
		return
	}
	returnJob, err := h.deliveryJobRepo.GetByID(c.Request.Context(), claimID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "claim not found"})
		return
	}

	if !returnJob.IsReturn || returnJob.Status != domain.DJStatusReturnDamageDisputed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a pending damage claim"})
		return
	}

	c.JSON(http.StatusOK, toReturnDamageClaimResponse(returnJob))
}

type ResolveReturnDamageClaimRequest struct {
	Decision        string   `json:"decision" binding:"required,oneof=full_refund partial_refund release_to_seller"`
	RefundAmount    *float64 `json:"refund_amount"`
	ResolutionNotes string   `json:"resolution_notes" binding:"required,min=10,max=2000"`
}

func (h *AdminHandler) ResolveReturnDamageClaim(c *gin.Context) {
	claimID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid claim ID"})
		return
	}

	adminID := c.MustGet("user_id").(uuid.UUID)

	var req ResolveReturnDamageClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	serviceReq := &service.ResolveDamageClaimRequest{
		Decision:        req.Decision,
		RefundAmount:    req.RefundAmount,
		ResolutionNotes: req.ResolutionNotes,
	}

	if err := h.disputeResolutionService.ResolveDamageClaim(c.Request.Context(), claimID, adminID, serviceReq); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "damage claim resolved successfully"})
}

type ReturnDamageClaimResponse struct {
	ID                uuid.UUID  `json:"id"`
	PaymentRequestID  uuid.UUID  `json:"payment_request_id"`
	OriginalJobID     *uuid.UUID `json:"original_job_id,omitempty"`
	DisputeID         *uuid.UUID `json:"dispute_id,omitempty"`
	Status            string     `json:"status"`
	ReturnCondition   *string    `json:"return_condition,omitempty"`
	DamageClaimReason *string    `json:"damage_claim_reason,omitempty"`
	PickupAddress     string     `json:"pickup_address"`
	DropoffAddress    string     `json:"dropoff_address"`
	RiderID           *uuid.UUID `json:"rider_id,omitempty"`
	CreatedAt         string     `json:"created_at"`
	ReturnConfirmedAt *string    `json:"return_confirmed_at,omitempty"`
}

func toReturnDamageClaimResponse(job *domain.DeliveryJob) ReturnDamageClaimResponse {
	resp := ReturnDamageClaimResponse{
		ID:                job.ID,
		PaymentRequestID:  job.PaymentRequestID,
		OriginalJobID:     job.OriginalJobID,
		DisputeID:         job.DisputeID,
		Status:            string(job.Status),
		ReturnCondition:   job.ReturnCondition,
		DamageClaimReason: job.DamageClaimReason,
		PickupAddress:     job.PickupAddress,
		DropoffAddress:    job.DropoffAddress,
		RiderID:           job.RiderID,
		CreatedAt:         job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if job.ReturnConfirmedAt != nil {
		t := job.ReturnConfirmedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.ReturnConfirmedAt = &t
	}

	return resp
}

func (h *AdminHandler) AddSupportStaff(c *gin.Context) {
	adminID := c.MustGet("user_id").(uuid.UUID)

	var req dto.AddSupportStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	staff, err := h.supportStaffService.AddStaff(c.Request.Context(), adminID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, staff)
}

func (h *AdminHandler) ListSupportStaff(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	activeOnly := c.Query("active_only") == "true"

	staff, total, err := h.supportStaffService.ListStaff(c.Request.Context(), activeOnly, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"support_staff": staff,
		"total":         total,
	})
}

func (h *AdminHandler) GetSupportStaff(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	staff, err := h.supportStaffService.GetStaff(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, staff)
}

func (h *AdminHandler) UpdateSupportStaff(c *gin.Context) {
	adminID := c.MustGet("user_id").(uuid.UUID)

	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req dto.UpdateSupportStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	staff, err := h.supportStaffService.UpdateStaff(c.Request.Context(), adminID, userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, staff)
}

func (h *AdminHandler) RemoveSupportStaff(c *gin.Context) {
	adminID := c.MustGet("user_id").(uuid.UUID)

	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	if err := h.supportStaffService.RemoveStaff(c.Request.Context(), adminID, userID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "support staff removed successfully"})
}

func (h *AdminHandler) ListAllTransactions(c *gin.Context) {
	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	response, err := h.adminService.ListAllTransactions(c.Request.Context(), nil, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AdminHandler) ListAllDeliveryJobs(c *gin.Context) {
	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	response, err := h.adminService.ListAllDeliveryJobs(c.Request.Context(), nil, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}
