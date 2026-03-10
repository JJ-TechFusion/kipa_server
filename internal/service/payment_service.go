package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/integration/flutterwave"
	"kipa/internal/integration/paystack"
	"kipa/internal/repository"
)
type OrderService interface {
	InitializePayment(ctx context.Context, buyerID uuid.UUID, paymentCode string, req *InitializePaymentRequest) (*InitializePaymentResponse, error)
	VerifyPayment(ctx context.Context, reference string) (*VerifyPaymentResponse, error)
	ProcessWalletPayment(ctx context.Context, buyerID uuid.UUID, paymentCode string) (*WalletPaymentResponse, error)
	GetOrderByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	GetOrdersByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Order, error)
	GetOrdersBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Order, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error
}

type InitializePaymentRequest struct {
	CallbackURL string `json:"callback_url"`
	Email       string `json:"email" binding:"required,email"`
}

type InitializePaymentResponse struct {
	OrderID          uuid.UUID `json:"order_id"`
	PaymentRequestID uuid.UUID `json:"payment_request_id"`
	AuthorizationURL string    `json:"authorization_url"`
	AccessCode       string    `json:"access_code"`
	Reference        string    `json:"reference"`
}

type VerifyPaymentResponse struct {
	OrderID             uuid.UUID  `json:"order_id"`
	PaymentRequestID    uuid.UUID  `json:"payment_request_id"`
	PaymentStatus       string     `json:"payment_status"`
	EscrowID            *uuid.UUID `json:"escrow_id,omitempty"`
	LogisticsDeliveryID *uuid.UUID `json:"logistics_delivery_id,omitempty"`
	Status              string     `json:"status"`
}

type WalletPaymentResponse struct {
	OrderID             uuid.UUID  `json:"order_id"`
	PaymentRequestID    uuid.UUID  `json:"payment_request_id"`
	EscrowID            uuid.UUID  `json:"escrow_id"`
	LogisticsDeliveryID *uuid.UUID `json:"logistics_delivery_id,omitempty"`
	Status              string     `json:"status"`
}

type OrderServiceImpl struct {
	orderRepo                repository.OrderRepository
	prRepo                   repository.PaymentRequestRepository
	userRepo                 repository.UserRepository
	escrowService            EscrowService
	walletService            WalletService
	logisticsDeliveryService LogisticsDeliveryService
	paystack                 *paystack.Client
	flutterwave              *flutterwave.Client
	notificationService      NotificationService
	config                   *config.Config
	logger                   *zap.Logger
}

func NewOrderService(
	orderRepo repository.OrderRepository,
	prRepo repository.PaymentRequestRepository,
	userRepo repository.UserRepository,
	escrowService EscrowService,
	walletService WalletService,
	logisticsDeliveryService LogisticsDeliveryService,
	notificationService NotificationService,
	paystackClient *paystack.Client,
	flutterwaveClient *flutterwave.Client,
	cfg *config.Config,
	logger *zap.Logger,
) OrderService {
	return &OrderServiceImpl{
		orderRepo:                orderRepo,
		prRepo:                   prRepo,
		userRepo:                 userRepo,
		escrowService:            escrowService,
		walletService:            walletService,
		logisticsDeliveryService: logisticsDeliveryService,
		notificationService:      notificationService,
		paystack:                 paystackClient,
		flutterwave:              flutterwaveClient,
		config:                   cfg,
		logger:                   logger,
	}
}
func (s *OrderServiceImpl) InitializePayment(ctx context.Context, buyerID uuid.UUID, paymentCode string, req *InitializePaymentRequest) (*InitializePaymentResponse, error) {
	pr, err := s.prRepo.GetByPaymentCode(ctx, paymentCode)
	if err != nil {
		return nil, err
	}
	if pr.Status != domain.PRStatusLinkActive {
		return nil, domain.ErrInvalidStatusTransition
	}
	if err := s.validateLinkUsage(pr); err != nil {
		return nil, err
	}
	deliveryFee := 0.0
	if pr.EstimatedDeliveryFee != nil {
		deliveryFee = *pr.EstimatedDeliveryFee
	}
	buyerFee := 0.0
	if pr.BuyerServiceFee != nil {
		buyerFee = *pr.BuyerServiceFee
	}
	totalAmount := pr.ItemPrice + buyerFee
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
	reference := fmt.Sprintf("kipa_ord_%s_%d", order.ID.String()[:8], time.Now().Unix())
	order.PaystackReference = &reference // Reusing field for Flutterwave tx_ref

	if err := s.orderRepo.Create(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	redirectURL := req.CallbackURL
	if redirectURL == "" {
		redirectURL = fmt.Sprintf("https://getkipa.com/pay/%s/verify?tx_ref=%s", paymentCode, reference)
	}
	flwReq := &flutterwave.InitializePaymentRequest{
		TxRef:       reference,
		Amount:      totalAmount,
		Currency:    "NGN",
		RedirectURL: redirectURL,
		Customer: flutterwave.CustomerInfo{
			Email: req.Email,
		},
		Customizations: &flutterwave.PaymentCustomization{
			Title:       "Kipa Payment",
			Description: fmt.Sprintf("Payment for %s", pr.ItemName),
		},
		Meta: map[string]interface{}{
			"order_id":           order.ID.String(),
			"payment_request_id": pr.ID.String(),
			"buyer_id":           buyerID.String(),
		},
	}

	flwResp, err := s.flutterwave.InitializePayment(ctx, flwReq)
	if err != nil {
		s.orderRepo.UpdatePaymentStatus(ctx, order.ID, domain.OrderPaymentFailed)
		return nil, fmt.Errorf("failed to initialize Flutterwave payment: %w", err)
	}

	paymentLink := flwResp.Data.Link
	order.PaystackAccessCode = &paymentLink // Reusing field for Flutterwave payment link
	order.PaymentStatus = domain.OrderPaymentInitialized
	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logger.Error("failed to update order with payment link", zap.Error(err))
	}

	s.logger.Info("payment initialized via Flutterwave",
		zap.String("order_id", order.ID.String()),
		zap.String("reference", reference),
	)

	return &InitializePaymentResponse{
		OrderID:          order.ID,
		PaymentRequestID: pr.ID,
		AuthorizationURL: paymentLink,
		AccessCode:       paymentLink, // For compatibility
		Reference:        reference,
	}, nil
}
func (s *OrderServiceImpl) VerifyPayment(ctx context.Context, reference string) (*VerifyPaymentResponse, error) {
	order, err := s.orderRepo.GetByPaystackReference(ctx, reference)
	if err != nil {
		return nil, err
	}
	if order.PaymentStatus == domain.OrderPaymentVerified {
		return &VerifyPaymentResponse{
			OrderID:          order.ID,
			PaymentRequestID: order.PaymentRequestID,
			PaymentStatus:    string(order.PaymentStatus),
			EscrowID:         order.EscrowID,
			Status:           string(order.Status),
		}, nil
	}
	if order.PaymentStatus != domain.OrderPaymentInitialized {
		return nil, domain.ErrOrderPaymentFailed
	}

	verification, err := s.flutterwave.VerifyTransaction(ctx, reference)
	if err != nil {
		s.logger.Error("Flutterwave verification failed", zap.Error(err), zap.String("reference", reference))
		return nil, domain.ErrPaymentVerificationFailed
	}
	if verification.Data.Status != "successful" {
		s.orderRepo.UpdatePaymentStatus(ctx, order.ID, domain.OrderPaymentFailed)
		return nil, domain.ErrPaymentNotSuccessful
	}
	if verification.Data.Amount != order.TotalAmount {
		s.logger.Error("payment amount mismatch",
			zap.Float64("expected", order.TotalAmount),
			zap.Float64("actual", verification.Data.Amount),
		)
		return nil, domain.ErrPaymentAmountMismatch
	}
	pr, err := s.prRepo.GetByID(ctx, order.PaymentRequestID)
	if err != nil {
		return nil, err
	}
	escrowReq := &CreateEscrowRequest{
		PaymentRequestID:    pr.ID,
		BuyerID:             order.BuyerID,
		SellerID:            pr.SellerID,
		ItemAmount:          order.ItemAmount,
		DeliveryFeeEstimate: order.DeliveryFeeEstimate,
		BuyerFee:            order.BuyerServiceFee,
		PaymentReference:    reference,
		PaymentProvider:     "flutterwave",
	}

	escrow, err := s.escrowService.CreateAndFund(ctx, escrowReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create escrow: %w", err)
	}

	var logisticsDeliveryID *uuid.UUID
	if pr.IsInterStateDelivery() && s.logisticsDeliveryService != nil {
		pickupAddress := ""
		if pr.PickupAddress != nil {
			pickupAddress = *pr.PickupAddress
		}
		dropoffAddress := ""
		if pr.DropoffAddress != nil {
			dropoffAddress = *pr.DropoffAddress
		}
		shipDeadline := time.Now().Add(72 * time.Hour)

		pickupState := ""
		if pr.PickupState != nil {
			pickupState = *pr.PickupState
		}
		dropoffState := ""
		if pr.DropoffState != nil {
			dropoffState = *pr.DropoffState
		}

		logisticsReq := &CreateLogisticsDeliveryRequest{
			PaymentRequestID: pr.ID,
			EscrowID:         escrow.ID,
			SellerID:         pr.SellerID,
			BuyerID:          order.BuyerID,
			PickupAddress:    pickupAddress,
			PickupState:      pickupState,
			DropoffAddress:   dropoffAddress,
			DropoffState:     dropoffState,
			ShipDeadline:     shipDeadline,
			EscrowAmount:     escrow.TotalLocked,
		}

		logisticsDelivery, err := s.logisticsDeliveryService.Create(ctx, logisticsReq)
		if err != nil {
			s.logger.Error("failed to create logistics delivery", zap.Error(err))
		} else {
			logisticsDeliveryID = &logisticsDelivery.ID
			pr.LogisticsDeliveryID = &logisticsDelivery.ID
			s.logger.Info("logistics delivery created",
				zap.String("logistics_delivery_id", logisticsDelivery.ID.String()),
				zap.String("pr_id", pr.ID.String()),
			)
		}
	}

	now := time.Now()
	order.EscrowID = &escrow.ID
	order.PaymentStatus = domain.OrderPaymentVerified
	order.Status = domain.OrderStatusPaid
	order.PaidAt = &now
	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logger.Error("failed to update order after verification", zap.Error(err))
	}
	pr.EscrowID = &escrow.ID
	pr.Status = domain.PRStatusPaidAwaitingFulfillment
	pr.BuyerID = &order.BuyerID
	pr.PaidAt = &now
	if err := s.prRepo.Update(ctx, pr); err != nil {
		s.logger.Error("failed to update payment request", zap.Error(err))
	}
	if err := s.prRepo.IncrementCurrentUses(ctx, pr.ID); err != nil {
		s.logger.Error("failed to increment current uses", zap.Error(err))
	}
	if err := s.userRepo.SetIsBuyer(ctx, order.BuyerID); err != nil {
		s.logger.Warn("failed to set user as buyer", zap.Error(err))
	}

	s.logger.Info("payment verified and escrow created",
		zap.String("order_id", order.ID.String()),
		zap.String("escrow_id", escrow.ID.String()),
		zap.String("reference", reference),
	)
	go func() {
		err := s.notificationService.SendToUser(
			context.Background(),
			pr.SellerID,
			"Payment Received",
			fmt.Sprintf("You have received a payment of %.2f for %s", order.TotalAmount, pr.ItemName),
			map[string]string{
				"order_id": order.ID.String(),
				"type":     string(domain.NotificationOrderPaid),
			},
			domain.NotificationOrderPaid,
		)
		if err != nil {
			s.logger.Warn("failed to notify seller of payment", zap.Error(err))
		}
	}()

	return &VerifyPaymentResponse{
		OrderID:             order.ID,
		PaymentRequestID:    order.PaymentRequestID,
		PaymentStatus:       string(order.PaymentStatus),
		EscrowID:            &escrow.ID,
		LogisticsDeliveryID: logisticsDeliveryID,
		Status:              string(order.Status),
	}, nil
}
func (s *OrderServiceImpl) ProcessWalletPayment(ctx context.Context, buyerID uuid.UUID, paymentCode string) (*WalletPaymentResponse, error) {
	pr, err := s.prRepo.GetByPaymentCode(ctx, paymentCode)
	if err != nil {
		return nil, err
	}
	if pr.Status != domain.PRStatusLinkActive {
		return nil, domain.ErrInvalidStatusTransition
	}
	if err := s.validateLinkUsage(pr); err != nil {
		return nil, err
	}
	deliveryFee := 0.0
	if pr.EstimatedDeliveryFee != nil {
		deliveryFee = *pr.EstimatedDeliveryFee
	}
	buyerFee := 0.0
	if pr.BuyerServiceFee != nil {
		buyerFee = *pr.BuyerServiceFee
	}
	totalAmount := pr.ItemPrice + buyerFee
	balance, err := s.walletService.GetBalance(ctx, buyerID)
	if err != nil {
		return nil, err
	}

	if balance.AvailableBalance < totalAmount {
		return nil, domain.ErrInsufficientBalance
	}
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
		s.orderRepo.UpdatePaymentStatus(ctx, order.ID, domain.OrderPaymentFailed)
		return nil, err
	}
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
		return nil, fmt.Errorf("failed to create escrow: %w", err)
	}
	var logisticsDeliveryID *uuid.UUID
	if pr.IsInterStateDelivery() && s.logisticsDeliveryService != nil {
		pickupAddress := ""
		if pr.PickupAddress != nil {
			pickupAddress = *pr.PickupAddress
		}
		dropoffAddress := ""
		if pr.DropoffAddress != nil {
			dropoffAddress = *pr.DropoffAddress
		}
		pickupState := ""
		if pr.PickupState != nil {
			pickupState = *pr.PickupState
		}
		dropoffState := ""
		if pr.DropoffState != nil {
			dropoffState = *pr.DropoffState
		}
		shipDeadline := time.Now().Add(72 * time.Hour)

		logisticsReq := &CreateLogisticsDeliveryRequest{
			PaymentRequestID: pr.ID,
			EscrowID:         escrow.ID,
			SellerID:         pr.SellerID,
			BuyerID:          buyerID,
			PickupAddress:    pickupAddress,
			PickupState:      pickupState,
			DropoffAddress:   dropoffAddress,
			DropoffState:     dropoffState,
			ShipDeadline:     shipDeadline,
			EscrowAmount:     escrow.TotalLocked,
		}

		logisticsDelivery, err := s.logisticsDeliveryService.Create(ctx, logisticsReq)
		if err != nil {
			s.logger.Error("failed to create logistics delivery", zap.Error(err))
		} else {
			logisticsDeliveryID = &logisticsDelivery.ID
			pr.LogisticsDeliveryID = &logisticsDelivery.ID
			s.logger.Info("logistics delivery created",
				zap.String("logistics_delivery_id", logisticsDelivery.ID.String()),
				zap.String("pr_id", pr.ID.String()),
			)
		}
	}

	now := time.Now()
	order.EscrowID = &escrow.ID
	order.PaymentStatus = domain.OrderPaymentVerified
	order.Status = domain.OrderStatusPaid
	order.PaidAt = &now
	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logger.Error("failed to update order", zap.Error(err))
	}
	pr.EscrowID = &escrow.ID
	pr.Status = domain.PRStatusPaidAwaitingFulfillment
	pr.BuyerID = &buyerID
	pr.PaidAt = &now
	if err := s.prRepo.Update(ctx, pr); err != nil {
		s.logger.Error("failed to update payment request", zap.Error(err))
	}
	if err := s.prRepo.IncrementCurrentUses(ctx, pr.ID); err != nil {
		s.logger.Error("failed to increment current uses", zap.Error(err))
	}
	if err := s.userRepo.SetIsBuyer(ctx, buyerID); err != nil {
		s.logger.Warn("failed to set user as buyer", zap.Error(err))
	}

	s.logger.Info("wallet payment processed",
		zap.String("order_id", order.ID.String()),
		zap.String("escrow_id", escrow.ID.String()),
	)
	go func() {
		err := s.notificationService.SendToUser(
			context.Background(),
			pr.SellerID,
			"Payment Received",
			fmt.Sprintf("You have received a wallet payment of %.2f for %s", order.TotalAmount, pr.ItemName),
			map[string]string{
				"order_id": order.ID.String(),
				"type":     string(domain.NotificationOrderPaid),
			},
			domain.NotificationOrderPaid,
		)
		if err != nil {
			s.logger.Warn("failed to notify seller of payment", zap.Error(err))
		}
	}()

	return &WalletPaymentResponse{
		OrderID:             order.ID,
		PaymentRequestID:    pr.ID,
		EscrowID:            escrow.ID,
		LogisticsDeliveryID: logisticsDeliveryID,
		Status:              string(order.Status),
	}, nil
}
func (s *OrderServiceImpl) GetOrderByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return s.orderRepo.GetByID(ctx, id)
}
func (s *OrderServiceImpl) GetOrdersByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	return s.orderRepo.GetByBuyerID(ctx, buyerID, limit, offset)
}
func (s *OrderServiceImpl) GetOrdersBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	return s.orderRepo.GetBySellerID(ctx, sellerID, limit, offset)
}
func (s *OrderServiceImpl) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	return s.orderRepo.UpdateStatus(ctx, id, status)
}
func (s *OrderServiceImpl) validateLinkUsage(pr *domain.PaymentRequest) error {
	if !pr.IsReusable && pr.CurrentUses > 0 {
		return domain.ErrLinkNotReusable
	}
	if pr.IsReusable && pr.MaxUses != nil && pr.CurrentUses >= *pr.MaxUses {
		return domain.ErrMaxUsesExceeded
	}

	return nil
}

var _ OrderService = (*OrderServiceImpl)(nil)
