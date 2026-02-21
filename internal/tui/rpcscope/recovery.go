package rpcscope

import (
	"strings"
)

func (c *Client) isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if IsNonRetryableScopeError(err) {
		return false
	}
	if c.policy.RetryableMatcher != nil {
		return c.policy.RetryableMatcher(err)
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, needle := range c.policy.RetryableErrors {
		if strings.Contains(msg, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return IsScopeUnavailable(err)
}
