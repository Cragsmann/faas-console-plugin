package config

import "time"

// DefaultSATokenExpiry is requested duration of validity of the requested ServiceAccount token
const DefaultSATokenExpiry int64 = 7 * 24 * 60 * 60 // 7 days

// TokenRefreshDivisor makes the refresh threshold one sixth of a token's lifetime.
const TokenRefreshDivisor = 6

// RefreshWindow is the remaining-lifetime threshold at which a token is refreshed.
func RefreshWindow(saTokenExpirySeconds int64) time.Duration {
	lifetime := time.Duration(saTokenExpirySeconds) * time.Second
	return lifetime / TokenRefreshDivisor
}
