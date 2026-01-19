package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/integration/paystack"
	"kipa/internal/repository"
)

// OrderService handles payment order operations
type OrderService interface {
	// InitializePayment creates an order and initializes payment with Paystack
	InitializePayment(ctx context.Context, buyerID uuid.UUID, paymentCode string, req *InitializePaymentRequest) (*InitializePaymentResponse, error)

	// VerifyPayment verifies payment with Paystack and creates escrow
	VerifyPayment(ctx context.Context, reference string) (*VerifyPaymentResponse, error)

	// ProcessWalletPayment processes wallet payment directly
	ProcessWalletPayment(ctx context.Context, buyerID uuid.UUID, paymentCode string) (*WalletPaymentResponse, error)

	// GetOrderByID gets an order by ID
	GetOrderByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)

	// GetOrdersByBuyerID gets all orders for a buyer
	GetOrdersByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Order, error)

	// GetOrdersBySellerID gets all orders for a seller (via payment requests)
	GetOrdersBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Order, error)

	// UpdateStatus updates the order status
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error
}

type InitializePaymentRequest struct {
	CallbackURL string `json:"callback_url"`
	Email       string `json:"email" binding:"required,email"`
}

type InitializePaymentResponse struct {
	OrderID          uuid.UUID `json:"order_id"`
	AuthorizationURL string    `json:"authorization_url"`
	AccessCode       string    `json:"access_code"`
	Reference        string    `json:"reference"`
}

type VerifyPaymentResponse struct {
	OrderID       uuid.UUID  `json:"order_id"`
	PaymentStatus string     `json:"payment_status"`
	EscrowID      *uuid.UUID `json:"escrow_id,omitempty"`
	Status        string     `json:"status"`
}

type WalletPaymentResponse struct {
	OrderID  uuid.UUID `json:"order_id"`
	EscrowID uuid.UUID `json:"escrow_id"`
	Status   string    `json:"status"`
}

type OrderServiceImpl struct {
	orderRepo     repository.OrderRepository
	prRepo        repository.PaymentRequestRepository
	userRepo      repository.UserRepository
	escrowService EscrowService
	walletService WalletService
	paystack      *paystack.Client
	config        *config.Config
	logger        *zap.Logger
}

func NewOrderService(
	orderRepo repository.OrderRepository,
	prRepo repository.PaymentRequestRepository,
	userRepo repository.UserRepository,
	escrowService EscrowService,
	walletService WalletService,
	paystackClient *paystack.Client,
	cfg *config.Config,
	logger *zap.Logger,
) OrderService {
	return &OrderServiceImpl{
		orderRepo:     orderRepo,
		prRepo:        prRepo,
		userRepo:      userRepo,
		escrowService: escrowService,
		walletService: walletService,
		paystack:      paystackClient,
		config:        cfg,
		logger:        logger,
	}
}

// InitializePayment creates an order and initializes payment with Paystack
func (s *OrderServiceImpl) InitializePayment(ctx context.Context, buyerID uuid.UUID, paymentCode string, req *InitializePaymentRequest) (*InitializePaymentResponse, error) {
	// Get payment request
	pr, err := s.prRepo.GetByPaymentCode(ctx, paymentCode)
	if err != nil {
		return nil, err
	}

	// Validate payment request status
	if pr.Status != domain.PRStatusLinkActive {
		return nil, domain.ErrInvalidStatusTransition
	}

	// Check link usage limits
	if err := s.validateLinkUsage(pr); err != nil {
		return nil, err
	}

	// Calculate amounts
	// Calculate amounts
	// Note: Delivery fee is not collected upfront, it is paid by buyer on delivery
	deliveryFee := 0.0
	if pr.EstimatedDeliveryFee != nil {
		deliveryFee = *pr.EstimatedDeliveryFee
	}
	buyerFee := 0.0
	if pr.BuyerServiceFee != nil {
		buyerFee = *pr.BuyerServiceFee
	}
	// Total amount to charge = Item Price + Buyer Service Fee
	totalAmount := pr.ItemPrice + buyerFee

	// Create order
	order := &domain.Order{
		ID:                  uuid.New(),
		PaymentRequestID:    pr.ID,
		BuyerID:             buyerID,
		PaymentMethod:       domain.PaymentMethodCard,
		PaymentStatus:       domain.OrderPaymentPending,
		ItemAmount:          pr.ItemPrice,
		DeliveryFeeEstimate: deliveryFee,
		BuyerServiceFee:     buyerFee,
		TotalAmount:         totalAmount,
		Status:              domain.OrderStatusPending,
	}

	// Generate Paystack reference
	reference := domain.GeneratePaystackReference(order.ID)
	order.PaystackReference = &reference

	if err := s.orderRepo.Create(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	// Initialize payment with Paystack
	amountKobo := int64(totalAmount * 100) // Convert to kobo
	paystackReq := &paystack.InitializeTransactionRequest{
		Email:       req.Email,
		Amount:      amountKobo,
		Reference:   reference,
		CallbackURL: req.CallbackURL,
		Metadata: map[string]interface{}{
			"order_id":           order.ID.String(),
			"payment_request_id": pr.ID.String(),
			"buyer_id":           buyerID.String(),
		},
	}

	paystackResp, err := s.paystack.InitializeTransaction(ctx, paystackReq)
	if err != nil {
		// Update order status to failed
		s.orderRepo.UpdatePaymentStatus(ctx, order.ID, domain.OrderPaymentFailed)
		return nil, fmt.Errorf("failed to initialize Paystack payment: %w", err)
	}

	// Update order with access code
	accessCode := paystackResp.Data.AccessCode
	order.PaystackAccessCode = &accessCode
	order.PaymentStatus = domain.OrderPaymentInitialized
	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logger.Error("failed to update order with access code", zap.Error(err))
	}

	s.logger.Info("payment initialized",
		zap.String("order_id", order.ID.String()),
		zap.String("reference", reference),
	)

	return &InitializePaymentResponse{
		OrderID:          order.ID,
		AuthorizationURL: paystackResp.Data.AuthorizationURL,
		AccessCode:       accessCode,
		Reference:        reference,
	}, nil
}

// VerifyPayment verifies payment with Paystack and creates escrow
func (s *OrderServiceImpl) VerifyPayment(ctx context.Context, reference string) (*VerifyPaymentResponse, error) {
	// Get order by reference
	order, err := s.orderRepo.GetByPaystackReference(ctx, reference)
	if err != nil {
		return nil, err
	}

	// Check if already verified
	if order.PaymentStatus == domain.OrderPaymentVerified {
		return &VerifyPaymentResponse{
			OrderID:       order.ID,
			PaymentStatus: string(order.PaymentStatus),
			EscrowID:      order.EscrowID,
			Status:        string(order.Status),
		}, nil
	}

	// Can only verify initialized payments
	if order.PaymentStatus != domain.OrderPaymentInitialized {
		return nil, domain.ErrOrderPaymentFailed
	}

	// Verify with Paystack
	verification, err := s.paystack.VerifyTransaction(ctx, reference)
	if err != nil {
		s.logger.Error("Paystack verification failed", zap.Error(err), zap.String("reference", reference))
		return nil, domain.ErrPaymentVerificationFailed
	}

	// Check if payment was successful
	if verification.Data.Status != "success" {
		s.orderRepo.UpdatePaymentStatus(ctx, order.ID, domain.OrderPaymentFailed)
		return nil, domain.ErrPaymentNotSuccessful
	}

	// Verify amount matches
	expectedKobo := int64(order.TotalAmount * 100)
	if verification.Data.Amount != expectedKobo {
		s.logger.Error("payment amount mismatch",
			zap.Int64("expected", expectedKobo),
			zap.Int64("actual", verification.Data.Amount),
		)
		return nil, domain.ErrPaymentAmountMismatch
	}

	// Get payment request for creating escrow
	pr, err := s.prRepo.GetByID(ctx, order.PaymentRequestID)
	if err != nil {
		return nil, err
	}

	// Create escrow
	escrowReq := &CreateEscrowRequest{
		PaymentRequestID:    pr.ID,
		BuyerID:             order.BuyerID,
		SellerID:            pr.SellerID,
		ItemAmount:          order.ItemAmount,
		DeliveryFeeEstimate: order.DeliveryFeeEstimate,
		BuyerFee:            order.BuyerServiceFee,
		PaymentReference:    reference,
		PaymentProvider:     "paystack",
	}

	escrow, err := s.escrowService.CreateAndFund(ctx, escrowReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create escrow: %w", err)
	}

	// Update order
	now := time.Now()
	order.EscrowID = &escrow.ID
	order.PaymentStatus = domain.OrderPaymentVerified
	order.Status = domain.OrderStatusPaid
	order.PaidAt = &now
	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logger.Error("failed to update order after verification", zap.Error(err))
	}

	// Link Escrow to Payment Request and update status
	pr.EscrowID = &escrow.ID
	pr.Status = domain.PRStatusPaidAwaitingFulfillment
	if err := s.prRepo.Update(ctx, pr); err != nil {
		s.logger.Error("failed to link escrow to payment request", zap.Error(err))
	}

	// Increment current uses on payment request
	if err := s.prRepo.IncrementCurrentUses(ctx, pr.ID); err != nil {
		s.logger.Error("failed to increment current uses", zap.Error(err))
	}

	// Update payment request with buyer (for single-use links) or leave as is for reusable
	if !pr.IsReusable {
		// For single-use, update the payment request status
		pr.Status = domain.PRStatusPaidAwaitingFulfillment
		pr.BuyerID = &order.BuyerID
		pr.EscrowID = &escrow.ID
		pr.PaidAt = &now
		if err := s.prRepo.Update(ctx, pr); err != nil {
			s.logger.Error("failed to update payment request", zap.Error(err))
		}
	}

	// Mark user as buyer (idempotent)
	if err := s.userRepo.SetIsBuyer(ctx, order.BuyerID); err != nil {
		s.logger.Warn("failed to set user as buyer", zap.Error(err))
	}

	s.logger.Info("payment verified and escrow created",
		zap.String("order_id", order.ID.String()),
		zap.String("escrow_id", escrow.ID.String()),
		zap.String("reference", reference),
	)

	return &VerifyPaymentResponse{
		OrderID:       order.ID,
		PaymentStatus: string(order.PaymentStatus),
		EscrowID:      &escrow.ID,
		Status:        string(order.Status),
	}, nil
}

// ProcessWalletPayment processes wallet payment directly
func (s *OrderServiceImpl) ProcessWalletPayment(ctx context.Context, buyerID uuid.UUID, paymentCode string) (*WalletPaymentResponse, error) {
	// Get payment request
	pr, err := s.prRepo.GetByPaymentCode(ctx, paymentCode)
	if err != nil {
		return nil, err
	}

	// Validate payment request status
	if pr.Status != domain.PRStatusLinkActive {
		return nil, domain.ErrInvalidStatusTransition
	}

	// Check link usage limits
	if err := s.validateLinkUsage(pr); err != nil {
		return nil, err
	}

	// Calculate amounts
	// Calculate amounts
	// Note: Delivery fee is not collected upfront
	deliveryFee := 0.0
	if pr.EstimatedDeliveryFee != nil {
		deliveryFee = *pr.EstimatedDeliveryFee
	}
	buyerFee := 0.0
	if pr.BuyerServiceFee != nil {
		buyerFee = *pr.BuyerServiceFee
	}
	// Total amount to charge = Item Price + Buyer Service Fee
	totalAmount := pr.ItemPrice + buyerFee

	// Check wallet balance
	balance, err := s.walletService.GetBalance(ctx, buyerID)
	if err != nil {
		return nil, err
	}

	if balance.AvailableBalance < totalAmount {
		return nil, domain.ErrInsufficientBalance
	}

	// Create order
	order := &domain.Order{
		ID:                  uuid.New(),
		PaymentRequestID:    pr.ID,
		BuyerID:             buyerID,
		PaymentMethod:       domain.PaymentMethodWallet,
		PaymentStatus:       domain.OrderPaymentPending,
		ItemAmount:          pr.ItemPrice,
		DeliveryFeeEstimate: deliveryFee,
		BuyerServiceFee:     buyerFee,
		TotalAmount:         totalAmount,
		Status:              domain.OrderStatusPending,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	// Debit wallet
	idempotencyKey := fmt.Sprintf("order_%s", order.ID.String())
	_, err = s.walletService.Debit(
		ctx,
		buyerID,
		totalAmount,
		domain.WalletTxEscrowFund,
		"order",
		order.ID,
		"Payment for "+pr.ItemName,
		idempotencyKey,
	)
	if err != nil {
		// Update order status to failed
		s.orderRepo.UpdatePaymentStatus(ctx, order.ID, domain.OrderPaymentFailed)
		return nil, err
	}

	// Create escrow
	escrowReq := &CreateEscrowRequest{
		PaymentRequestID:    pr.ID,
		BuyerID:             buyerID,
		SellerID:            pr.SellerID,
		ItemAmount:          order.ItemAmount,
		DeliveryFeeEstimate: order.DeliveryFeeEstimate,
		BuyerFee:            order.BuyerServiceFee,
		PaymentReference:    idempotencyKey,
		PaymentProvider:     "wallet",
	}

	escrow, err := s.escrowService.CreateAndFund(ctx, escrowReq)
	if err != nil {
		// TODO: Refund wallet on escrow creation failure
		return nil, fmt.Errorf("failed to create escrow: %w", err)
	}

	// Update order
	now := time.Now()
	order.EscrowID = &escrow.ID
	order.PaymentStatus = domain.OrderPaymentVerified
	order.Status = domain.OrderStatusPaid
	order.PaidAt = &now
	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logger.Error("failed to update order", zap.Error(err))
	}

	// Link Escrow to Payment Request and update status
	pr.EscrowID = &escrow.ID
	pr.Status = domain.PRStatusPaidAwaitingFulfillment
	if err := s.prRepo.Update(ctx, pr); err != nil {
		s.logger.Error("failed to link escrow to payment request", zap.Error(err))
	}

	// Increment current uses
	if err := s.prRepo.IncrementCurrentUses(ctx, pr.ID); err != nil {
		s.logger.Error("failed to increment current uses", zap.Error(err))
	}

	// Update payment request for single-use links
	if !pr.IsReusable {
		pr.Status = domain.PRStatusPaidAwaitingFulfillment
		pr.BuyerID = &buyerID
		pr.EscrowID = &escrow.ID
		pr.PaidAt = &now
		if err := s.prRepo.Update(ctx, pr); err != nil {
			s.logger.Error("failed to update payment request", zap.Error(err))
		}
	}

	// Mark user as buyer (idempotent)
	if err := s.userRepo.SetIsBuyer(ctx, buyerID); err != nil {
		s.logger.Warn("failed to set user as buyer", zap.Error(err))
	}

	s.logger.Info("wallet payment processed",
		zap.String("order_id", order.ID.String()),
		zap.String("escrow_id", escrow.ID.String()),
	)

	return &WalletPaymentResponse{
		OrderID:  order.ID,
		EscrowID: escrow.ID,
		Status:   string(order.Status),
	}, nil
}

// GetOrderByID gets an order by ID
func (s *OrderServiceImpl) GetOrderByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return s.orderRepo.GetByID(ctx, id)
}

// GetOrdersByBuyerID gets all orders for a buyer
func (s *OrderServiceImpl) GetOrdersByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	return s.orderRepo.GetByBuyerID(ctx, buyerID, limit, offset)
}

// GetOrdersBySellerID gets all orders for a seller (via payment requests)
func (s *OrderServiceImpl) GetOrdersBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	return s.orderRepo.GetBySellerID(ctx, sellerID, limit, offset)
}

// UpdateStatus updates the order status
func (s *OrderServiceImpl) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	return s.orderRepo.UpdateStatus(ctx, id, status)
}

// validateLinkUsage checks if the payment link can still be used
func (s *OrderServiceImpl) validateLinkUsage(pr *domain.PaymentRequest) error {
	// Check if single-use link has already been used
	if !pr.IsReusable && pr.CurrentUses > 0 {
		return domain.ErrLinkNotReusable
	}

	// Check if reusable link has reached max uses
	if pr.IsReusable && pr.MaxUses != nil && pr.CurrentUses >= *pr.MaxUses {
		return domain.ErrMaxUsesExceeded
	}

	return nil
}

// Ensure OrderServiceImpl implements OrderService
var _ OrderService = (*OrderServiceImpl)(nil)
