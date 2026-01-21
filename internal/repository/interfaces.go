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
	TotalRiders     int
	VerifiedRiders  int
	PendingRiders   int
	SuspendedRiders int
	OnlineRiders    int
	AverageRating   float64
	TotalDeliveries int
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
