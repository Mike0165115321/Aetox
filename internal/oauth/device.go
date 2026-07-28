package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// pollDeviceCode never asks more often than this, whatever the provider says.
// RFC 8628 makes 5s the floor and a faster client gets rate-limited into
// slow_down anyway. A variable rather than a constant so the tests can drive
// the state machine without spending a minute doing it.
var pollFloor = 5 * time.Second

// pollDeviceCode runs the wait half of a device-code flow: the user is off
// typing a short code into a web page, and we ask the token endpoint every few
// seconds whether they finished.
//
// Shared by Copilot and Qwen because RFC 8628 is genuinely the same on both
// sides — including the two error strings that are not failures:
// authorization_pending (keep waiting) and slow_down (back off, and the
// interval increase is required, not advisory).
func pollDeviceCode(
	ctx context.Context,
	pending *Pending,
	endpoint string,
	form url.Values,
	post func(context.Context, string, url.Values, any) error,
) (tokenResponse, error) {
	interval := time.Duration(pending.interval) * time.Second
	if interval < pollFloor {
		interval = pollFloor
	}
	expiresIn := pending.expiresIn
	if expiresIn <= 0 {
		expiresIn = 900
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return tokenResponse{}, ctx.Err()
		case <-ticker.C:
		}

		var tokens tokenResponse
		// A transport error here is not fatal: the endpoint is being polled a
		// dozen times and one dropped connection should not throw away a login
		// the user has already approved.
		if err := post(ctx, endpoint, form, &tokens); err != nil {
			continue
		}

		switch tokens.Error {
		case "":
			if tokens.AccessToken == "" {
				continue
			}
			return tokens, nil
		case "authorization_pending":
			continue
		case "slow_down":
			interval += pollFloor
			ticker.Reset(interval)
		case "expired_token":
			return tokenResponse{}, errors.New("the sign-in code expired — start again")
		case "access_denied":
			return tokenResponse{}, errors.New("sign-in was denied")
		default:
			detail := tokens.ErrorDesc
			if detail == "" {
				detail = tokens.Error
			}
			return tokenResponse{}, fmt.Errorf("sign-in failed: %s", detail)
		}
	}
	return tokenResponse{}, errors.New("the sign-in code expired — start again")
}
