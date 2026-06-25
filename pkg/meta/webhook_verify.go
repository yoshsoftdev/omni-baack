package meta

import "errors"

var ErrWebhookVerificationFailed = errors.New("webhook verification failed")

// VerifyWebhookSubscription validates Meta webhook challenge handshake.
func VerifyWebhookSubscription(mode, token, challenge, expectedVerifyToken string) (string, error) {
	if mode != "subscribe" || token != expectedVerifyToken {
		return "", ErrWebhookVerificationFailed
	}
	if challenge == "" {
		return "", ErrWebhookVerificationFailed
	}
	return challenge, nil
}
