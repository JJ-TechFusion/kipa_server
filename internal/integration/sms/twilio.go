package sms

import (
	"context"

	"go.uber.org/zap"

	"github.com/twilio/twilio-go"
	verify "github.com/twilio/twilio-go/rest/verify/v2"

	"kipa/internal/config"
	"kipa/internal/integration"
)


type TwilioVerifyClient struct {
	client           *twilio.RestClient
	verifyServiceSID string
	logger           *zap.Logger
}


func NewTwilioVerifyClient(cfg *config.TwilioConfig, logger *zap.Logger) *TwilioVerifyClient {
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: cfg.AccountSID,
		Password: cfg.AuthToken,
	})

	return &TwilioVerifyClient{
		client:           client,
		verifyServiceSID: cfg.VerifyServiceSID,
		logger:           logger,
	}
}


func (c *TwilioVerifyClient) SendVerification(ctx context.Context, phoneNumber, channel string) (*integration.VerifyResult, error) {
	params := &verify.CreateVerificationParams{}
	params.SetTo(phoneNumber)
	params.SetChannel(channel) // "sms" or "call"

	resp, err := c.client.VerifyV2.CreateVerification(c.verifyServiceSID, params)
	if err != nil {
		c.logger.Error("failed to send verification via Twilio",
			zap.String("phone", phoneNumber),
			zap.String("channel", channel),
			zap.Error(err),
		)
		return &integration.VerifyResult{
			Success: false,
			Error:   err,
		}, err
	}

	status := ""
	sid := ""
	if resp.Status != nil {
		status = *resp.Status
	}
	if resp.Sid != nil {
		sid = *resp.Sid
	}

	c.logger.Info("verification sent successfully via Twilio",
		zap.String("phone", phoneNumber),
		zap.String("channel", channel),
		zap.String("status", status),
		zap.String("sid", sid),
	)

	return &integration.VerifyResult{
		Success:         true,
		Status:          status,
		VerificationSID: sid,
	}, nil
}

func (c *TwilioVerifyClient) CheckVerification(ctx context.Context, phoneNumber, code string) (*integration.VerifyResult, error) {
	params := &verify.CreateVerificationCheckParams{}
	params.SetTo(phoneNumber)
	params.SetCode(code)

	resp, err := c.client.VerifyV2.CreateVerificationCheck(c.verifyServiceSID, params)
	if err != nil {
		c.logger.Error("failed to check verification via Twilio",
			zap.String("phone", phoneNumber),
			zap.Error(err),
		)
		return &integration.VerifyResult{
			Success: false,
			Error:   err,
		}, err
	}

	status := ""
	sid := ""
	if resp.Status != nil {
		status = *resp.Status
	}
	if resp.Sid != nil {
		sid = *resp.Sid
	}

	success := status == "approved"

	c.logger.Info("verification check completed",
		zap.String("phone", phoneNumber),
		zap.String("status", status),
		zap.Bool("success", success),
	)

	return &integration.VerifyResult{
		Success:         success,
		Status:          status,
		VerificationSID: sid,
	}, nil
}

var _ integration.VerifyProvider = (*TwilioVerifyClient)(nil)
