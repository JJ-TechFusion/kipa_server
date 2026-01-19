package domain

import "errors"

var (
	ErrInvalidOTP             = errors.New("invalid or expired OTP")
	ErrOTPExpired             = errors.New("OTP has expired")
	ErrMaxOTPAttemptsExceeded = errors.New("maximum OTP verification attempts exceeded")
	ErrOTPRateLimitExceeded   = errors.New("too many OTP requests, please try again later")
	ErrOTPAlreadyVerified     = errors.New("OTP already verified")
	ErrOTPNotFound            = errors.New("OTP verification not found")
	ErrInvalidToken        = errors.New("invalid or expired token")
	ErrTokenExpired        = errors.New("token has expired")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrSessionNotFound     = errors.New("session not found")
	ErrSessionRevoked      = errors.New("session has been revoked")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserSuspended       = errors.New("user account is suspended")
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrPhoneNumberRequired = errors.New("phone number is required")
	ErrInvalidPhoneNumber  = errors.New("invalid phone number format")
	ErrProfileIncomplete   = errors.New("profile incomplete: first name, last name, and email are required")
	ErrIdempotencyKeyExists = errors.New("idempotency key already exists")
	ErrSMSDeliveryFailed   = errors.New("failed to send SMS")
	ErrVoiceDeliveryFailed = errors.New("failed to make voice call")
	ErrInsufficientBalance       = errors.New("insufficient balance")
	ErrInsufficientLockedBalance = errors.New("insufficient locked balance")
	ErrWalletNotFound            = errors.New("wallet not found")
	ErrRiderNotFound        = errors.New("rider not found")
	ErrRiderProfileNotFound = errors.New("rider profile not found")
	ErrRiderNotVerified     = errors.New("rider is not verified")
	ErrRiderNotOnline       = errors.New("rider is not online")
	ErrRiderAlreadyOnline   = errors.New("rider is already online")
	ErrRiderBusy            = errors.New("rider is busy with another delivery")
	ErrRiderBlockedForCommission        = errors.New("rider blocked: pay outstanding commission to continue accepting jobs")
	ErrPartialCommissionPaymentNotAllowed = errors.New("partial payment not allowed: must pay full outstanding commission")
	ErrNoCommissionOwed                 = errors.New("no outstanding commission to pay")
	ErrCommissionNotFound               = errors.New("commission record not found")
	ErrUnauthorized = errors.New("unauthorized access")
	ErrPaymentRequestNotFound   = errors.New("payment request not found")
	ErrPaymentRequestExpired    = errors.New("payment request has expired")
	ErrPaymentRequestCancelled  = errors.New("payment request was cancelled")
	ErrPaymentRequestPaid       = errors.New("payment request already paid")
	ErrInvalidStatusTransition  = errors.New("invalid status transition")
	ErrPaymentCodeAlreadyExists = errors.New("payment code already exists")
	ErrCannotCancel              = errors.New("cannot cancel payment request at this stage")
	ErrPaymentRequestNotEditable = errors.New("payment request cannot be edited in current status")
	ErrDeliveryJobNotFound = errors.New("delivery job not found")
	ErrJobAlreadyAccepted  = errors.New("job has already been accepted")
	ErrJobNotSearching     = errors.New("job is not in searching status")
	ErrInvalidPickupCode   = errors.New("invalid pickup code")
	ErrInvalidDropoffCode  = errors.New("invalid dropoff code")
	ErrInvalidVehicleType  = errors.New("invalid vehicle type: must be 'motorcycle' or 'car'")
	ErrVehicleTypeMismatch = errors.New("your vehicle type does not match the job requirements")
	ErrErrandNotFound       = errors.New("errand not found")
	ErrErrandAlreadyConfirmed = errors.New("errand already confirmed")
	ErrErrandNotConfirmable = errors.New("errand cannot be confirmed in current state")
	ErrErrandNotCancellable = errors.New("errand cannot be cancelled in current state")
	ErrEscrowNotFound    = errors.New("escrow not found")
	ErrEscrowNotFunded   = errors.New("escrow is not funded")
	ErrEscrowReleased    = errors.New("escrow has already been released")
	ErrEscrowDisputed    = errors.New("escrow is under dispute")
	ErrCannotDispute     = errors.New("cannot dispute at this stage")
	ErrZoneNotFound      = errors.New("delivery zone not found")
	ErrOutsideCoverage   = errors.New("location is outside delivery coverage")
	ErrNoRidersAvailable = errors.New("no riders available in the area")
	ErrPaymentVerificationFailed = errors.New("payment verification failed")
	ErrPaymentNotSuccessful      = errors.New("payment was not successful")
	ErrPaymentAmountMismatch     = errors.New("payment amount does not match expected amount")
	ErrInvalidPaymentReference   = errors.New("invalid payment reference")
	ErrBankAccountNotFound     = errors.New("bank account not found")
	ErrBankAccountExists       = errors.New("bank account already exists")
	ErrInvalidBankAccount      = errors.New("invalid bank account")
	ErrBankAccountNotVerified  = errors.New("bank account not verified")
	ErrVirtualAccountNotFound  = errors.New("virtual account not found")
	ErrVirtualAccountExists    = errors.New("virtual account already exists")
	ErrWithdrawalNotFound      = errors.New("withdrawal not found")
	ErrWithdrawalMinimum       = errors.New("withdrawal amount below minimum")
	ErrWithdrawalPending       = errors.New("withdrawal already pending")
	ErrTransferFailed          = errors.New("transfer to bank failed")
	ErrTopUpNotFound           = errors.New("top-up not found")
	ErrTopUpMinimum            = errors.New("top-up amount below minimum")
	ErrOrderNotFound          = errors.New("order not found")
	ErrOrderAlreadyPaid       = errors.New("order already paid")
	ErrOrderPaymentFailed     = errors.New("order payment failed")
	ErrMaxUsesExceeded        = errors.New("payment link max uses exceeded")
	ErrLinkNotReusable        = errors.New("payment link is not reusable and has already been used")
)
const (
	CodeInvalidInput   = "INVALID_INPUT"
	CodeInvalidOTP     = "INVALID_OTP"
	CodeOTPExpired     = "OTP_EXPIRED"
	CodeOTPMaxAttempts = "OTP_MAX_ATTEMPTS"
	CodeOTPRateLimit   = "OTP_RATE_LIMITED"
	CodeInvalidToken   = "INVALID_TOKEN"
	CodeTokenExpired   = "TOKEN_EXPIRED"
	CodeSessionRevoked = "SESSION_REVOKED"
	CodeUserNotFound   = "USER_NOT_FOUND"
	CodeUserSuspended  = "USER_SUSPENDED"
	CodeInternalError  = "INTERNAL_ERROR"
	CodeSMSFailed      = "SMS_DELIVERY_FAILED"
	CodeUnauthorized   = "UNAUTHORIZED"
)

type AppError struct {
	Err     error
	Message string
	Code    string
	Status  int
}

func (e *AppError) Error() string {
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewAppError(err error, message, code string, status int) *AppError {
	return &AppError{
		Err:     err,
		Message: message,
		Code:    code,
		Status:  status,
	}
}
