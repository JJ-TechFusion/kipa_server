package paystack
const (
	EventChargeSuccess     = "charge.success"
	EventTransferSuccess   = "transfer.success"
	EventTransferFailed    = "transfer.failed"
	EventTransferReversed  = "transfer.reversed"
	EventRefundProcessed   = "refund.processed"
	EventRefundFailed      = "refund.failed"
)
func IsPaymentSuccessEvent(eventType string) bool {
	return eventType == EventChargeSuccess
}
func IsTransferEvent(eventType string) bool {
	switch eventType {
	case EventTransferSuccess, EventTransferFailed, EventTransferReversed:
		return true
	default:
		return false
	}
}
func IsRefundEvent(eventType string) bool {
	switch eventType {
	case EventRefundProcessed, EventRefundFailed:
		return true
	default:
		return false
	}
}
