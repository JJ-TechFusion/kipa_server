package integration

import (
	"context"
)

type VerifyProvider interface {
	SendVerification(ctx context.Context, phoneNumber, channel string) (*VerifyResult, error)
	

	CheckVerification(ctx context.Context, phoneNumber, code string) (*VerifyResult, error)
}

type VerifyResult struct {
	Success         bool
	Status          string 
	VerificationSID string
	Error           error
}
