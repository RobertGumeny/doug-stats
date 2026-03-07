// Package pricing provides the model pricing registry and cost computation.
//
// Cache tier assumption: Claude's prompt caching operates on a 5-minute
// sliding window. CacheRead tokens are charged at the cache-read rate only
// when the cache hit occurs within this 5-minute tier. This assumption is
// returned as metadata (CacheTierMinutes = 5) for UI display.
package pricing

import (
	"math"

	"github.com/robertgumeny/doug-stats/provider"
)

// CacheTierMinutes is the assumed duration of Claude's prompt cache sliding
// window. Tokens hit within this window are charged at the CacheRead rate
// rather than the full Input rate. Returned as metadata for UI display.
const CacheTierMinutes = 5

// ModelPricing holds per-million-token USD rates for a model.
// Fields that do not apply to a model are zero.
type ModelPricing struct {
	InputPerMToken         float64 // USD per 1M input tokens
	CacheCreationPerMToken float64 // USD per 1M cache-write tokens
	CacheReadPerMToken     float64 // USD per 1M cache-read tokens
	OutputPerMToken        float64 // USD per 1M output tokens

	// GeminiSingleCachedPerMToken is the flat cached-token rate for Gemini
	// models. Zero for non-Gemini models.
	GeminiSingleCachedPerMToken float64

	// CodexReasoningPerMToken is the additional reasoning-token surcharge for
	// Codex models. Zero for non-Codex models.
	CodexReasoningPerMToken float64
}

// Registry maps model identifier strings to their pricing configuration.
// All rates are USD per one million tokens. Add entries here when new
// models are released; do not hardcode rates elsewhere.
var Registry = map[string]ModelPricing{
	// Claude 4 family
	"claude-opus-4-6": {
		InputPerMToken:         15.00,
		CacheCreationPerMToken: 18.75,
		CacheReadPerMToken:     1.50,
		OutputPerMToken:        75.00,
	},
	"claude-sonnet-4-6": {
		InputPerMToken:         3.00,
		CacheCreationPerMToken: 3.75,
		CacheReadPerMToken:     0.30,
		OutputPerMToken:        15.00,
	},
	"claude-haiku-4-5": {
		InputPerMToken:         0.80,
		CacheCreationPerMToken: 1.00,
		CacheReadPerMToken:     0.08,
		OutputPerMToken:        4.00,
	},
	// Claude 3.5 family
	"claude-3-5-sonnet-20241022": {
		InputPerMToken:         3.00,
		CacheCreationPerMToken: 3.75,
		CacheReadPerMToken:     0.30,
		OutputPerMToken:        15.00,
	},
	"claude-3-5-haiku-20241022": {
		InputPerMToken:         0.80,
		CacheCreationPerMToken: 1.00,
		CacheReadPerMToken:     0.08,
		OutputPerMToken:        4.00,
	},
}

// Cost is the computed USD cost for a set of token counts.
type Cost struct {
	USD     float64 // cost in USD, rounded to 4 decimal places; 0 when Unknown is true
	Unknown bool    // true when the model identifier is not in Registry
}

// Add returns the sum of two Costs. If either operand is Unknown, the result
// is Unknown.
func (c Cost) Add(other Cost) Cost {
	if c.Unknown || other.Unknown {
		return Cost{Unknown: true}
	}
	return Cost{USD: math.Round((c.USD+other.USD)*10000) / 10000}
}

// Compute calculates the USD cost for the given model and token counts,
// rounded to 4 decimal places. If the model is not in Registry, it returns
// Cost{Unknown: true} — never zero or a panic.
func Compute(model string, tokens provider.TokenCounts) Cost {
	p, ok := Registry[model]
	if !ok {
		return Cost{Unknown: true}
	}
	usd := (float64(tokens.Input)*p.InputPerMToken +
		float64(tokens.CacheCreation)*p.CacheCreationPerMToken +
		float64(tokens.CacheRead)*p.CacheReadPerMToken +
		float64(tokens.Output)*p.OutputPerMToken) / 1_000_000
	return Cost{USD: math.Round(usd*10000) / 10000}
}
