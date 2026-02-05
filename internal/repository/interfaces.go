package repository

import (
	"context"
	"time"

	"kipa/internal/domain"

	"github.com/google/uuid"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByPhoneNumber(ctx context.Context, phoneNumber string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	UpdateLastLogin(ctx context.Context, userID uuid.UUID) error
	SetPhoneVerified(ctx context.Context, userID uuid.UUID) error
	SetIsSeller(ctx context.Context, userID uuid.UUID) error
	SetIsBuyer(ctx context.Context, userID uuid.UUID) error
	SetIsRider(ctx context.Context, userID uuid.UUID) error
	
	// Admin methods
	Suspend(ctx context.Context, userID uuid.UUID, reason string) error
	Unsuspend(ctx context.Context, userID uuid.UUID) error
	VerifySeller(ctx context.Context, userID uuid.UUID) error
	List(ctx context.Context, filters interface{}, limit, offset int) ([]*domain.User, error)
	GetStatistics(ctx context.Context) (*UserStatistics, error)
}

type UserStatistics struct {
	TotalUsers     int `db:"total_users" json:"total_users"`
	TotalSellers   int `db:"total_sellers" json:"total_sellers"`
	TotalBuyers    int `db:"total_buyers" json:"total_buyers"`
	TotalRiders    int `db:"total_riders" json:"total_riders"`
	SuspendedUsers int `db:"suspended_users" json:"suspended_users"`
}

type OTPRepository interface {
	Create(ctx context.Context, otp *domain.OTPVerification) error
	GetByVerificationID(ctx context.Context, phoneNumber string, verificationID uuid.UUID) (*domain.OTPVerification, error)
	IncrementAttempts(ctx context.Context, id uuid.UUID) error
	MarkVerified(ctx context.Context, id uuid.UUID) error
	GetRecentCount(ctx context.Context, phoneNumber string, since time.Time) (int, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

type IdempotencyRepository interface {
	Create(ctx context.Context, key *domain.IdempotencyKey) error
	Exists(ctx context.Context, key string) (bool, error)
	GetByKey(ctx context.Context, key string) (*domain.IdempotencyKey, error)
}

type SessionRepository interface {
	Store(ctx context.Context, session *domain.Session) error
	Get(ctx context.Context, userID uuid.UUID, deviceID string) (*domain.Session, error)
	Delete(ctx context.Context, userID uuid.UUID, deviceID string) error
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
	ValidateRefreshToken(ctx context.Context, userID uuid.UUID, deviceID, refreshToken string) (bool, error)
	GetRateLimitCount(ctx context.Context, phoneNumber string) (int, error)
	IncrementRateLimitCount(ctx context.Context, phoneNumber string, window time.Duration) error
	GetLastOTPSentTime(ctx context.Context, phoneNumber string) (time.Time, error)
	SetLastOTPSentTime(ctx context.Context, phoneNumber string, ttl time.Duration) error
}
type PaymentRequestRepository interface {
	Create(ctx context.Context, pr *domain.PaymentRequest) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.PaymentRequest, error)
	GetByPaymentCode(ctx context.Context, code string) (*domain.PaymentRequest, error)
	GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error)
	GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error)
	GetHistoryBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error)
	GetExpiredLinks(ctx context.Context) ([]*domain.PaymentRequest, error)
	GetOverdueProcessingOrders(ctx context.Context, reminderCooldownMins int) ([]*domain.PaymentRequest, error)
	GetAll(ctx context.Context, limit, offset int) ([]*domain.PaymentRequest, error)
	Update(ctx context.Context, pr *domain.PaymentRequest) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.PaymentRequestStatus) error
	UpdateReminderSent(ctx context.Context, id uuid.UUID) error
	SetBuyer(ctx context.Context, id uuid.UUID, buyerID uuid.UUID) error
	SetEscrowID(ctx context.Context, id uuid.UUID, escrowID uuid.UUID) error
	SetDeliveryJobID(ctx context.Context, id uuid.UUID, jobID uuid.UUID) error
	PaymentCodeExists(ctx context.Context, code string) (bool, error)
	IncrementCurrentUses(ctx context.Context, id uuid.UUID) error

	// Unavailability handling
	GetExpiredRebookRequests(ctx context.Context) ([]*domain.PaymentRequest, error)
	SetUnavailabilityFee(ctx context.Context, id uuid.UUID, fee float64) error
}
type PaymentRequestEventRepository interface {
	Create(ctx context.Context, event *domain.PaymentRequestEvent) error
	GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) ([]*domain.PaymentRequestEvent, error)
}
type DeliveryJobRepository interface {
	Create(ctx context.Context, job *domain.DeliveryJob) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DeliveryJob, error)
	GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.DeliveryJob, error)
	GetByRiderID(ctx context.Context, riderID uuid.UUID, statuses []domain.DeliveryJobStatus, limit, offset int) ([]*domain.DeliveryJob, error)
	GetActiveByRiderID(ctx context.Context, riderID uuid.UUID) (*domain.DeliveryJob, error)
	GetSearchingJobs(ctx context.Context, limit int) ([]*domain.DeliveryJob, error)
	GetSearchingJobsByVehicleType(ctx context.Context, vehicleType domain.VehicleType, limit int) ([]*domain.DeliveryJob, error)
	GetTimedOutJobs(ctx context.Context) ([]*domain.DeliveryJob, error)
	GetJobsForRadiusExpansion(ctx context.Context, expandAfterMins int) ([]*domain.DeliveryJob, error)
	GetCompletedTodayByRiderID(ctx context.Context, riderID uuid.UUID) (int, error)
	Update(ctx context.Context, job *domain.DeliveryJob) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DeliveryJobStatus) error
	UpdateSearchRadius(ctx context.Context, id uuid.UUID, radiusKM int) error
	AssignRider(ctx context.Context, jobID, riderID uuid.UUID) error

	// Return delivery queries
	GetByStatus(ctx context.Context, status domain.DeliveryJobStatus, limit, offset int) ([]*domain.DeliveryJob, int, error)
	GetReturnJobByOriginalID(ctx context.Context, originalJobID uuid.UUID) (*domain.DeliveryJob, error)
	GetJobCoordinates(ctx context.Context, jobID uuid.UUID) (*JobCoordinates, error)
}

type JobCoordinates struct {
	PickupLat  float64
	PickupLng  float64
	DropoffLat float64
	DropoffLng float64
}
type RiderJobOfferRepository interface {
	Create(ctx context.Context, offer *domain.RiderJobOffer) error
	GetByJobID(ctx context.Context, jobID uuid.UUID) ([]*domain.RiderJobOffer, error)
	GetByRiderID(ctx context.Context, riderID uuid.UUID, response *domain.RiderOfferResponse) ([]*domain.RiderJobOffer, error)
	UpdateResponse(ctx context.Context, id uuid.UUID, response domain.RiderOfferResponse) error
	ExpirePendingOffers(ctx context.Context, jobID uuid.UUID) error
	ExpireAllStaleOffers(ctx context.Context, olderThanMins int) (int64, error)
}
type EscrowRepository interface {
	Create(ctx context.Context, escrow *domain.Escrow) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Escrow, error)
	GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.Escrow, error)
	GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Escrow, error)
	GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Escrow, error)
	Update(ctx context.Context, escrow *domain.Escrow) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.EscrowStatus) error
	GetPendingConfirmationWindow(ctx context.Context) ([]*domain.Escrow, error)
}
type WalletRepository interface {
	Create(ctx context.Context, wallet *domain.Wallet) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error)
	GetOrCreate(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error)
	Update(ctx context.Context, wallet *domain.Wallet) error
	Credit(ctx context.Context, walletID uuid.UUID, amount float64) error
	Debit(ctx context.Context, walletID uuid.UUID, amount float64) error
}
type WalletTransactionRepository interface {
	Create(ctx context.Context, tx *domain.WalletTransaction) error
	GetByWalletID(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]*domain.WalletTransaction, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.WalletTransaction, error)
	IdempotencyKeyExists(ctx context.Context, key string) (bool, error)
	GetRevenueAnalytics(ctx context.Context, startDate, endDate *time.Time) (*RevenueAnalytics, error)
}

type RevenueAnalytics struct {
	TotalRevenue      float64 `db:"total_revenue" json:"total_revenue"`
	BuyerFees         float64 `db:"buyer_fees" json:"buyer_fees"`
	SellerFees        float64 `db:"seller_fees" json:"seller_fees"`
	DeliveryFees      float64 `db:"delivery_fees" json:"delivery_fees"`
	TotalTransactions int     `db:"total_transactions" json:"total_transactions"`
}

type RiderProfileRepository interface {
	Create(ctx context.Context, profile *domain.RiderProfile) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.RiderProfile, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.RiderProfile, error)
	Update(ctx context.Context, profile *domain.RiderProfile) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.RiderStatus) error
	SetOnline(ctx context.Context, id uuid.UUID, isOnline bool) error
	UpdateLocation(ctx context.Context, id uuid.UUID, lat, lng float64) error
	GetOnlineRidersInRadius(ctx context.Context, lat, lng float64, radiusKM int) ([]*domain.RiderProfile, error)
	IncrementDeliveries(ctx context.Context, id uuid.UUID) error
	AddEarnings(ctx context.Context, id uuid.UUID, amount float64) error
	UpdateCommissionStats(ctx context.Context, id uuid.UUID, unpaidCommission float64, unpaidCount int) error
	SetCommissionBlocked(ctx context.Context, id uuid.UUID, blocked bool) error
	ClearCommissionDebt(ctx context.Context, id uuid.UUID) error
	RecordCommissionPayment(ctx context.Context, id uuid.UUID) error
	
	// Admin methods
	GetByStatus(ctx context.Context, status domain.RiderStatus, limit, offset int) ([]*domain.RiderProfile, error)
	GetStatistics(ctx context.Context) (*RiderStatistics, error)
}

type RiderStatistics struct {
	TotalRiders     int     `db:"total_riders"`
	VerifiedRiders  int     `db:"verified_riders"`
	PendingRiders   int     `db:"pending_riders"`
	SuspendedRiders int     `db:"suspended_riders"`
	OnlineRiders    int     `db:"online_riders"`
	AverageRating   float64 `db:"average_rating"`
	TotalDeliveries int     `db:"total_deliveries"`
}
type RiderCommissionRepository interface {
	Create(ctx context.Context, commission *domain.RiderCommission) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.RiderCommission, error)
	GetByDeliveryJobID(ctx context.Context, jobID uuid.UUID) (*domain.RiderCommission, error)
	GetPendingByRiderID(ctx context.Context, riderProfileID uuid.UUID) ([]*domain.RiderCommission, error)
	GetByRiderID(ctx context.Context, riderProfileID uuid.UUID, limit, offset int) ([]*domain.RiderCommission, error)
	GetPendingTotals(ctx context.Context, riderProfileID uuid.UUID) (totalAmount float64, count int, err error)
	MarkAllPendingAsPaid(ctx context.Context, riderProfileID uuid.UUID, paidVia string) (int, error)
	MarkAsWaived(ctx context.Context, riderProfileID uuid.UUID, paidVia string) (int, error)
}
type CommissionPaymentRepository interface {
	Create(ctx context.Context, payment *domain.CommissionPayment) error
	GetByRiderID(ctx context.Context, riderProfileID uuid.UUID, limit, offset int) ([]*domain.CommissionPayment, error)
}
type DeliveryZoneRepository interface {
	Create(ctx context.Context, zone *domain.DeliveryZone) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DeliveryZone, error)
	GetByLocation(ctx context.Context, lat, lng float64) (*domain.DeliveryZone, error)
	GetAll(ctx context.Context) ([]*domain.DeliveryZone, error)
	GetActive(ctx context.Context) ([]*domain.DeliveryZone, error)
	Update(ctx context.Context, zone *domain.DeliveryZone) error
}
type BankAccountRepository interface {
	Create(ctx context.Context, account *domain.BankAccount) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.BankAccount, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.BankAccount, error)
	GetDefaultByUserID(ctx context.Context, userID uuid.UUID) (*domain.BankAccount, error)
	GetByAccountNumber(ctx context.Context, userID uuid.UUID, accountNumber, bankCode string) (*domain.BankAccount, error)
	Update(ctx context.Context, account *domain.BankAccount) error
	SetDefault(ctx context.Context, userID, accountID uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
}
type VirtualAccountRepository interface {
	Create(ctx context.Context, account *domain.VirtualAccount) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.VirtualAccount, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.VirtualAccount, error)
	GetByAccountNumber(ctx context.Context, accountNumber string) (*domain.VirtualAccount, error)
	GetByCustomerCode(ctx context.Context, customerCode string) (*domain.VirtualAccount, error)
	Update(ctx context.Context, account *domain.VirtualAccount) error
}
type WithdrawalRepository interface {
	Create(ctx context.Context, withdrawal *domain.Withdrawal) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Withdrawal, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Withdrawal, error)
	GetByReference(ctx context.Context, reference string) (*domain.Withdrawal, error)
	GetPendingByUserID(ctx context.Context, userID uuid.UUID) (*domain.Withdrawal, error)
	Update(ctx context.Context, withdrawal *domain.Withdrawal) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WithdrawalStatus) error
}
type TopUpRepository interface {
	Create(ctx context.Context, topUp *domain.TopUp) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.TopUp, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.TopUp, error)
	GetByReference(ctx context.Context, reference string) (*domain.TopUp, error)
	Update(ctx context.Context, topUp *domain.TopUp) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.TopUpStatus) error
}
type OrderRepository interface {
	Create(ctx context.Context, order *domain.Order) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	GetByPaystackReference(ctx context.Context, reference string) (*domain.Order, error)
	GetByPaymentRequestID(ctx context.Context, prID uuid.UUID, limit, offset int) ([]*domain.Order, error)
	GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Order, error)
	GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Order, error)
	Update(ctx context.Context, order *domain.Order) error
	UpdatePaymentStatus(ctx context.Context, id uuid.UUID, status domain.OrderPaymentStatus) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error
	SetEscrowID(ctx context.Context, id uuid.UUID, escrowID uuid.UUID) error
}
type ErrandRepository interface {
	Create(ctx context.Context, errand *domain.Errand) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Errand, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Errand, error)
	GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Errand, error)
	Update(ctx context.Context, errand *domain.Errand) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ErrandStatus) error
	AssignRider(ctx context.Context, errandID, riderID, jobID uuid.UUID) error
	SetDeliveryJobID(ctx context.Context, errandID, jobID uuid.UUID) error
}
type ChatMessageRepository interface {
	Create(ctx context.Context, msg *domain.ChatMessage) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ChatMessage, error)
	GetByJobID(ctx context.Context, jobID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*domain.ChatMessage, error)
	MarkDelivered(ctx context.Context, id uuid.UUID) error
	MarkReadByReceiver(ctx context.Context, jobID, receiverID uuid.UUID) error
	GetUnreadCount(ctx context.Context, jobID, userID uuid.UUID) (int, error)
}

type LogisticsDeliveryRepository interface {
	Create(ctx context.Context, delivery *domain.LogisticsDelivery) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.LogisticsDelivery, error)
	GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.LogisticsDelivery, error)
	GetByEscrowID(ctx context.Context, escrowID uuid.UUID) (*domain.LogisticsDelivery, error)
	GetBySellerID(ctx context.Context, sellerID uuid.UUID, status *domain.LogisticsDeliveryStatus, limit, offset int) ([]*domain.LogisticsDelivery, int, error)
	GetByBuyerID(ctx context.Context, buyerID uuid.UUID, status *domain.LogisticsDeliveryStatus, limit, offset int) ([]*domain.LogisticsDelivery, int, error)
	GetByStatus(ctx context.Context, status domain.LogisticsDeliveryStatus, limit, offset int) ([]*domain.LogisticsDelivery, int, error)
	Update(ctx context.Context, delivery *domain.LogisticsDelivery) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.LogisticsDeliveryStatus) error

	// Deadline queries
	GetExceededShipDeadlines(ctx context.Context) ([]*domain.LogisticsDelivery, error)
	GetPendingConfirmationReminders(ctx context.Context, reminderIntervalHours int) ([]*domain.LogisticsDelivery, error)
	GetReadyForAutoEscalation(ctx context.Context) ([]*domain.LogisticsDelivery, error)
	GetPendingAdminApproval(ctx context.Context, limit, offset int) ([]*domain.LogisticsDelivery, int, error)

	// Update specific fields
	SetShipped(ctx context.Context, id uuid.UUID, carrier *domain.LogisticsCarrier, carrierOtherName, trackingNumber *string, trackingURL, shipmentReceiptURL *string, estimatedDeliveryAt *time.Time) error
	SetDeliveryClaimed(ctx context.Context, id uuid.UUID, confirmationDeadline, autoEscalationAt time.Time) error
	SetConfirmed(ctx context.Context, id uuid.UUID, requiresAdminApproval bool) error
	SetDispute(ctx context.Context, id uuid.UUID, disputeID uuid.UUID) error
	SetReturnInfo(ctx context.Context, id uuid.UUID, buyerShare, sellerShare float64) error
	SetReturnShipped(ctx context.Context, id uuid.UUID, carrier, trackingNumber string) error
	UpdateReminderSent(ctx context.Context, id uuid.UUID) error
}

type LogisticsDeliveryEventRepository interface {
	Create(ctx context.Context, event *domain.LogisticsDeliveryEvent) error
	GetByDeliveryID(ctx context.Context, deliveryID uuid.UUID) ([]*domain.LogisticsDeliveryEvent, error)
}

type LogisticsEvidenceRepository interface {
	Create(ctx context.Context, evidence *domain.LogisticsEvidence) error
	GetByDeliveryID(ctx context.Context, deliveryID uuid.UUID) ([]*domain.LogisticsEvidence, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.LogisticsEvidence, error)
}

// ============================================
// INTER-STATE DRIVER DELIVERY REPOSITORIES
// ============================================

type DriverProfileRepository interface {
	Create(ctx context.Context, profile *domain.DriverProfile) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DriverProfile, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.DriverProfile, error)
	Update(ctx context.Context, profile *domain.DriverProfile) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DriverStatus) error

	// Capacity management
	UpdateCapacity(ctx context.Context, id uuid.UUID, carryValue float64, packageCount int) error
	ResetCapacity(ctx context.Context, id uuid.UUID) error

	// Trip management
	SetCurrentTrip(ctx context.Context, id uuid.UUID, tripID *uuid.UUID, origin, destination *string) error

	// Stats
	IncrementDeliveries(ctx context.Context, id uuid.UUID) error
	AddEarnings(ctx context.Context, id uuid.UUID, amount float64) error

	// Queries
	GetAvailableForRoute(ctx context.Context, originCity, destCity string) ([]*domain.DriverProfile, error)
	GetByStatus(ctx context.Context, status domain.DriverStatus, limit, offset int) ([]*domain.DriverProfile, error)
	GetPending(ctx context.Context, limit, offset int) ([]*domain.DriverProfile, error)
	GetStatistics(ctx context.Context) (*DriverStatistics, error)
}

type DriverStatistics struct {
	TotalDrivers    int     `db:"total_drivers" json:"total_drivers"`
	VerifiedDrivers int     `db:"verified_drivers" json:"verified_drivers"`
	OnlineDrivers   int     `db:"online_drivers" json:"online_drivers"`
	InTransitDrivers int    `db:"in_transit_drivers" json:"in_transit_drivers"`
	TotalDeliveries int     `db:"total_deliveries" json:"total_deliveries"`
	AverageRating   float64 `db:"average_rating" json:"average_rating"`
}

type DriverTripRepository interface {
	Create(ctx context.Context, trip *domain.DriverTrip) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DriverTrip, error)
	GetByDriverID(ctx context.Context, driverID uuid.UUID) (*domain.DriverTrip, error)
	GetActiveByDriverID(ctx context.Context, driverID uuid.UUID) (*domain.DriverTrip, error)
	GetByStatus(ctx context.Context, status domain.DriverTripStatus, limit, offset int) ([]*domain.DriverTrip, error)
	GetByRoute(ctx context.Context, originCity, destCity string, status *domain.DriverTripStatus, limit, offset int) ([]*domain.DriverTrip, error)
	GetCompletedByDriverID(ctx context.Context, driverID uuid.UUID, limit, offset int) ([]*domain.DriverTrip, error)
	Update(ctx context.Context, trip *domain.DriverTrip) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DriverTripStatus) error

	// Aggregates
	UpdateAggregates(ctx context.Context, id uuid.UUID, totalPackages int, totalValue, totalDriverFee float64) error
	AddDelivery(ctx context.Context, id uuid.UUID, itemValue, driverFee float64) error
	RemoveDelivery(ctx context.Context, id uuid.UUID, itemValue, driverFee float64) error

	// Timeline
	MarkDeparted(ctx context.Context, id uuid.UUID) error
	MarkArrived(ctx context.Context, id uuid.UUID) error
	MarkCompleted(ctx context.Context, id uuid.UUID) error

	// Monitor queries
	GetStaleDepartures(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error)
	GetAbandonedTrips(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error)
}

type DriverDeliveryRepository interface {
	Create(ctx context.Context, delivery *domain.DriverDelivery) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DriverDelivery, error)
	GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.DriverDelivery, error)
	GetByTripID(ctx context.Context, tripID uuid.UUID) ([]*domain.DriverDelivery, error)
	GetByDriverID(ctx context.Context, driverID uuid.UUID, statuses []domain.DriverDeliveryStatus, limit, offset int) ([]*domain.DriverDelivery, error)
	GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error)
	GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error)
	Update(ctx context.Context, delivery *domain.DriverDelivery) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DriverDeliveryStatus) error

	// Queries for matching
	GetAwaitingDriver(ctx context.Context, originCity, destCity string, limit int) ([]*domain.DriverDelivery, error)
	GetAvailableForRoute(ctx context.Context, originCity, destCity string, maxValue float64, limit int) ([]*domain.DriverDelivery, error)

	// Assignment
	AssignDriver(ctx context.Context, id uuid.UUID, driverID, tripID uuid.UUID) error

	// Code management
	SetPickupCode(ctx context.Context, id uuid.UUID, code string) error
	SetDropoffCode(ctx context.Context, id uuid.UUID, code string) error
	IncrementPickupAttempts(ctx context.Context, id uuid.UUID) error
	IncrementDropoffAttempts(ctx context.Context, id uuid.UUID) error

	// Timeline updates
	MarkAtPickup(ctx context.Context, id uuid.UUID) error
	MarkPickedUp(ctx context.Context, id uuid.UUID) error
	MarkDeparted(ctx context.Context, id uuid.UUID) error
	MarkArrived(ctx context.Context, id uuid.UUID) error
	MarkDelivered(ctx context.Context, id uuid.UUID) error
	MarkCompleted(ctx context.Context, id uuid.UUID) error

	// Buyer unreachable
	MarkBuyerUnreachable(ctx context.Context, id uuid.UUID, deadline time.Time) error
	GetExpiredUnreachable(ctx context.Context) ([]*domain.DriverDelivery, error)

	// Bulk operations for trip status changes
	UpdateStatusByTripID(ctx context.Context, tripID uuid.UUID, fromStatuses []domain.DriverDeliveryStatus, toStatus domain.DriverDeliveryStatus) error

	// Monitor queries
	GetExpiredAwaiting(ctx context.Context, hoursThreshold int) ([]*domain.DriverDelivery, error)
	GetStaleInTransit(ctx context.Context, hoursThreshold int) ([]*domain.DriverDelivery, error)
	GetExpiredConfirmationWindow(ctx context.Context) ([]*domain.DriverDelivery, error)
}

type DriverDeliveryEventRepository interface {
	Create(ctx context.Context, event *domain.DriverDeliveryEvent) error
	GetByDeliveryID(ctx context.Context, deliveryID uuid.UUID) ([]*domain.DriverDeliveryEvent, error)
}

type DriverOfferRepository interface {
	Create(ctx context.Context, offer *domain.DriverOffer) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DriverOffer, error)
	Update(ctx context.Context, offer *domain.DriverOffer) error

	// Query methods
	GetByDeliveryID(ctx context.Context, deliveryID uuid.UUID) ([]*domain.DriverOffer, error)
	GetByDriverID(ctx context.Context, driverID uuid.UUID, status *domain.DriverOfferStatus) ([]*domain.DriverOffer, error)
	GetPendingByDeliveryAndDriver(ctx context.Context, deliveryID, driverID uuid.UUID) (*domain.DriverOffer, error)
	GetOfferCountByDeliveryAndDriver(ctx context.Context, deliveryID, driverID uuid.UUID) (int, error)

	// Bulk operations
	RejectAllPendingExcept(ctx context.Context, deliveryID uuid.UUID, exceptOfferID uuid.UUID) error
	ExpireStaleOffers(ctx context.Context) (int64, error)
	ExpireStaleCounterOffers(ctx context.Context) (int64, error)
}
