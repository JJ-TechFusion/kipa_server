package paystack

// Webhook event types
const (
	EventChargeSuccess     = "charge.success"
	EventTransferSuccess   = "transfer.success"
	EventTransferFailed    = "transfer.failed"
	EventTransferReversed  = "transfer.reversed"
	EventRefundProcessed   = "refund.processed"
	EventRefundFailed      = "refund.failed"
)

// IsPaymentSuccessEvent checks if the event is a successful payment event
func IsPaymentSuccessEvent(eventType string) bool {
	return eventType == EventChargeSuccess
}

// IsTransferEvent checks if the event is a transfer-related event
func IsTransferEvent(eventType string) bool {
	switch eventType {
	case EventTransferSuccess, EventTransferFailed, EventTransferReversed:
		return true
	default:
		return false
	}
}

// IsRefundEvent checks if the event is a refund-related event
func IsRefundEvent(eventType string) bool {
	switch eventType {
	case EventRefundProcessed, EventRefundFailed:
		return true
	default:
		return false
	}
}
