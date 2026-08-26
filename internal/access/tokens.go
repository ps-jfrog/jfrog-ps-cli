package access

import (
	"fmt"
	"time"

	"jfrogpscli/internal/jfclient"
)

// Token is one access token entry from GET /access/api/v1/tokens.
type Token struct {
	TokenID string `json:"token_id"`
	Subject string `json:"subject"`
	Expiry  int64  `json:"expiry"` // unix seconds; 0 means non-expiring
	Scope   string `json:"scope"`
}

// ExpiringToken is a token flagged by TokenExpiry, with its expiry resolved
// to a time.Time for display.
type ExpiringToken struct {
	TokenID   string
	Subject   string
	ExpiresAt time.Time
}

// ListTokens fetches all access tokens via GET /access/api/v1/tokens.
func ListTokens(c *jfclient.Client) ([]Token, error) {
	resp, err := c.Get("/access/api/v1/tokens")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list tokens"); err != nil {
		return nil, err
	}
	var wrapper struct {
		Tokens []Token `json:"tokens"`
	}
	if err := resp.Unmarshal(&wrapper); err != nil {
		return nil, fmt.Errorf("parse tokens list: %w", err)
	}
	return wrapper.Tokens, nil
}

// TokenExpiry lists all tokens and reports those expiring within days of
// now (or already expired). Tokens with expiry == 0 (non-expiring) are
// always skipped. now is passed explicitly rather than read via time.Now()
// internally, so callers can get deterministic results in tests.
func TokenExpiry(c *jfclient.Client, days int, now time.Time) ([]ExpiringToken, error) {
	tokens, err := ListTokens(c)
	if err != nil {
		return nil, err
	}
	cutoff := now.Add(time.Duration(days) * 24 * time.Hour)
	var out []ExpiringToken
	for _, t := range tokens {
		if t.Expiry == 0 {
			continue
		}
		exp := time.Unix(t.Expiry, 0)
		if exp.Before(cutoff) || exp.Equal(cutoff) {
			out = append(out, ExpiringToken{TokenID: t.TokenID, Subject: t.Subject, ExpiresAt: exp})
		}
	}
	return out, nil
}
