package integration

import (
	"context"
)

type SMSProvider interface {
	SendSMS(ctx context.Context, phoneNumber, message string) (*SMSResult, error)
	SendVoiceOTP(ctx context.Context, phoneNumber, otp string) (*SMSResult, error)
	SendWithFallback(ctx context.Context, phoneNumber, otp string) (*SMSResult, error)
}

type SMSResult struct {
	Success   bool
	MessageID string
	Provider  string
	Error     error
}
