package fcm

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

type Client struct {
	app       *firebase.App
	msgClient *messaging.Client
	logger    *zap.Logger
}


func NewClient(credentialsPath string, logger *zap.Logger) (*Client, error) {
	var app *firebase.App
	var err error

	ctx := context.Background()

	if credentialsPath != "" {
		opt := option.WithCredentialsFile(credentialsPath)
		app, err = firebase.NewApp(ctx, nil, opt)
	} else {
		// Uses GOOGLE_APPLICATION_CREDENTIALS environment variable
		app, err = firebase.NewApp(ctx, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase app: %w", err)
	}

	msgClient, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get messaging client: %w", err)
	}

	return &Client{
		app:       app,
		msgClient: msgClient,
		logger:    logger,
	}, nil
}

// SendToToken sends a notification to a specific device token
func (c *Client) SendToToken(ctx context.Context, token, title, body string, data map[string]string) error {
	message := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default",
				},
			},
		},
	}

	response, err := c.msgClient.Send(ctx, message)
	if err != nil {
		c.logger.Error("failed to send FCM message",
			zap.String("token", token[:20]+"..."),
			zap.Error(err),
		)
		return err
	}

	c.logger.Debug("FCM message sent",
		zap.String("message_id", response),
	)
	return nil
}

// SendToTokens sends a notification to multiple device tokens
func (c *Client) SendToTokens(ctx context.Context, tokens []string, title, body string, data map[string]string) (*messaging.BatchResponse, error) {
	if len(tokens) == 0 {
		return nil, nil
	}

	message := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default",
				},
			},
		},
	}

	response, err := c.msgClient.SendEachForMulticast(ctx, message)
	if err != nil {
		c.logger.Error("failed to send multicast FCM message", zap.Error(err))
		return nil, err
	}

	c.logger.Info("FCM multicast sent",
		zap.Int("success", response.SuccessCount),
		zap.Int("failure", response.FailureCount),
	)

	return response, nil
}

// SendToTopic sends a notification to all devices subscribed to a topic
func (c *Client) SendToTopic(ctx context.Context, topic, title, body string, data map[string]string) error {
	message := &messaging.Message{
		Topic: topic,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
	}

	response, err := c.msgClient.Send(ctx, message)
	if err != nil {
		c.logger.Error("failed to send FCM topic message",
			zap.String("topic", topic),
			zap.Error(err),
		)
		return err
	}

	c.logger.Debug("FCM topic message sent",
		zap.String("topic", topic),
		zap.String("message_id", response),
	)
	return nil
}
