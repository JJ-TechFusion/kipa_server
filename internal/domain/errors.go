package domain

import "errors"

var (
	ErrInvalidOTP             = errors.New("invalid or expired OTP")
	ErrOTPExpired             = errors.New("OTP has expired")
	ErrMaxOTPAttemptsExceeded = errors.New("maximum OTP verification attempts exceeded")
	ErrOTPRateLimitExceeded   = errors.New("too many OTP requests, please try again later")
	ErrOTPResendCooldown      = errors.New("please wait before requesting another OTP")
	ErrOTPAlreadyVerified     = errors.New("OTP already verified")
	ErrOTPNotFound            = errors.New("OTP verification not found")
	ErrInvalidToken        = errors.New("invalid or expired token")
	ErrTokenExpired        = errors.New("token has expired")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrSessionNotFound     = errors.New("session not found")
	ErrSessionRevoked      = errors.New("session has been revoked")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserSuspended         = errors.New("user account is suspended")
	ErrUserAlreadyExists     = errors.New("user already exists")
	ErrActiveEscrowsExist    = errors.New("cannot delete account with active escrows")
	ErrActiveDeliveriesExist = errors.New("cannot delete account with active deliveries")
	ErrWalletBalanceNotZero  = errors.New("cannot delete account with wallet balance")
	ErrPhoneNumberRequired = errors.New("phone number is required")
	ErrInvalidPhoneNumber  = errors.New("invalid phone number format")
	ErrProfileIncomplete   = errors.New("profile incomplete: first name, last name, and email are required")
	ErrIdempotencyKeyExists  = errors.New("idempotency key already exists")
	ErrInvalidAdminSecret    = errors.New("invalid admin signup secret")
	ErrAdminSignupNotAllowed = errors.New("admin signup is not configured")
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
	ErrMissingInterStateFields = errors.New("pickup_state and dropoff_state are required for inter-state delivery")
	ErrLogisticsDeliveryNotFound     = errors.New("logistics delivery not found")
	ErrInvalidCarrier                = errors.New("invalid logistics carrier")
	ErrTrackingNumberRequired        = errors.New("tracking number is required")
	ErrShipmentReceiptRequired       = errors.New("shipment receipt is required")
	ErrCannotShipYet                 = errors.New("cannot mark as shipped in current state")
	ErrCannotClaimDelivery           = errors.New("cannot claim delivery in current state")
	ErrCannotClaimDeliveryTooEarly   = errors.New("cannot claim delivery within 24 hours of shipping")
	ErrCannotClaimReturnDelivery     = errors.New("cannot claim return delivery in current state")
	ErrCannotConfirmDelivery         = errors.New("cannot confirm delivery in current state")
	ErrCannotCancelAfterShipment     = errors.New("cannot cancel after item has been shipped")
	ErrShipDeadlineExceeded          = errors.New("ship deadline has been exceeded")
	ErrCannotOpenDisputeInState      = errors.New("cannot open dispute in current state")
	ErrSellerCannotDisputeYet        = errors.New("seller can only open dispute after 2 days of buyer non-response")
	ErrDisputeAlreadyOpen            = errors.New("a dispute is already open for this delivery")
	ErrDisputeNotFound               = errors.New("dispute not found")
	ErrReturnTrackingRequired        = errors.New("return tracking number is required")
	ErrHighValueRequiresAdminApproval = errors.New("high value orders require admin approval before release")
	ErrDriverNotFound            = errors.New("driver not found")
	ErrDriverProfileNotFound     = errors.New("driver profile not found")
	ErrDriverProfileExists       = errors.New("driver profile already exists")
	ErrDriverNotVerified         = errors.New("driver is not verified")
	ErrDriverNotOnline           = errors.New("driver is not online")
	ErrDriverAlreadyOnline       = errors.New("driver is already online")
	ErrDriverHasActiveTrip       = errors.New("driver has an active trip")
	ErrDriverCapacityExceeded    = errors.New("driver capacity exceeded")
	ErrDriverTripNotFound        = errors.New("driver trip not found")
	ErrDriverDeliveryNotFound    = errors.New("driver delivery not found")
	ErrInvalidDriverStatus       = errors.New("invalid driver status for this action")
	ErrDriverCannotAccept        = errors.New("driver cannot accept deliveries in current state")
	ErrDriverNotAccepting        = errors.New("driver is not accepting deliveries")
	ErrNoActiveTrip              = errors.New("no active trip found")
	ErrDeliveryAlreadyAssigned   = errors.New("delivery already assigned to a driver")
	ErrTripCannotDepart          = errors.New("trip cannot depart: no packages or wrong status")
	ErrTripNotInTransit          = errors.New("trip is not in transit")
	ErrPickupCodeLocked          = errors.New("pickup code attempts exceeded")
	ErrDropoffCodeLocked         = errors.New("dropoff code attempts exceeded")
	ErrPickupCodeExpired         = errors.New("pickup code has expired")
	ErrDropoffCodeExpired        = errors.New("dropoff code has expired")
	ErrDeliveryNotCancellable    = errors.New("delivery cannot be cancelled after pickup")
	ErrRouteMismatch             = errors.New("delivery route does not match driver route")
	ErrOfferNotFound              = errors.New("driver offer not found")
	ErrOfferExpired               = errors.New("driver offer has expired")
	ErrDriverAlreadyOffered       = errors.New("driver has already made an offer for this delivery")
	ErrDeliveryNotAcceptingOffers = errors.New("delivery is not accepting offers")
	ErrMaxOfferAttemptsExceeded   = errors.New("maximum offer attempts exceeded")
	ErrOfferCannotBeAccepted      = errors.New("offer cannot be accepted in current state")
	ErrOfferCannotBeRejected      = errors.New("offer cannot be rejected in current state")
	ErrOfferCannotBeCountered     = errors.New("offer cannot be countered in current state")
	ErrOfferCannotBeWithdrawn     = errors.New("offer cannot be withdrawn in current state")
	ErrCounterOfferExpired        = errors.New("counter offer has expired")
	ErrPinAlreadySet    = errors.New("wallet PIN already set")
	ErrPinNotSet        = errors.New("wallet PIN not set")
	ErrInvalidPin       = errors.New("invalid PIN")
	ErrPinLocked        = errors.New("wallet locked due to too many failed attempts")
	ErrPinTooShort      = errors.New("PIN must be at least 4 digits")
	ErrPinTooLong       = errors.New("PIN must be at most 6 digits")
	ErrPinMustBeNumeric     = errors.New("PIN must contain only digits")
	ErrPinResetNotRequested = errors.New("PIN reset not requested")
	ErrFlutterwaveSubaccountNotFound = errors.New("flutterwave subaccount not found")
	ErrFlutterwaveSubaccountExists   = errors.New("flutterwave subaccount already exists")
	ErrFlutterwaveAPIError           = errors.New("flutterwave API error")
	ErrFlutterwaveSyncFailed         = errors.New("failed to sync wallet with flutterwave subaccount")
)
const (
	CodeInvalidInput   = "INVALID_INPUT"
	CodeInvalidOTP     = "INVALID_OTP"
	CodeOTPExpired     = "OTP_EXPIRED"
	CodeOTPMaxAttempts = "OTP_MAX_ATTEMPTS"
	CodeOTPRateLimit   = "OTP_RATE_LIMITED"
	CodeOTPCooldown    = "OTP_COOLDOWN"
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
