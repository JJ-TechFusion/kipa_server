package util

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/google/uuid"
)

type OTPGenerator interface {
	Generate(length int) (string, error)
	GenerateVerificationID() uuid.UUID
}

type SecureOTPGenerator struct{}

func NewSecureOTPGenerator() *SecureOTPGenerator {
	return &SecureOTPGenerator{}
}

func (g *SecureOTPGenerator) Generate(length int) (string, error) {
	if length <= 0 {
		length = 6
	}

	otp := ""
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("failed to generate OTP: %w", err)
		}
		otp += n.String()
	}

	return otp, nil
}

func (g *SecureOTPGenerator) GenerateVerificationID() uuid.UUID {
	return uuid.New()
}
