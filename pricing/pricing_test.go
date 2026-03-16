package pricing

import (
	"testing"

	"github.com/robertgumeny/doug-stats/provider"
)

// --- Registry completeness ---

func TestRegistry_FiveClaudeModels(t *testing.T) {
	expected := []string{
		"claude-opus-4-6",
		"claude-sonnet-4-6",
		"claude-haiku-4-5",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
	}
	for _, model := range expected {
		if _, ok := Registry[model]; !ok {
			t.Errorf("model %q missing from Registry", model)
		}
	}
	if len(Registry) < len(expected) {
		t.Errorf("Registry has %d entries, want at least %d", len(Registry), len(expected))
	}
}

func TestRegistry_FiveGeminiModels(t *testing.T) {
	expected := []string{
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-2.0-flash",
		"gemini-3-flash-preview",
	}
	for _, model := range expected {
		if _, ok := Registry[model]; !ok {
			t.Errorf("model %q missing from Registry", model)
		}
	}
}

func TestRegistry_ThreeCodexModels(t *testing.T) {
	expected := []string{
		"gpt-5-codex",
		"gpt-5.1-codex",
		"codex-mini-latest",
	}
	for _, model := range expected {
		if _, ok := Registry[model]; !ok {
			t.Errorf("model %q missing from Registry", model)
		}
	}
}

func TestRegistry_GeminiAndCodexFields(t *testing.T) {
	// ModelPricing struct must have both fields (compile-time check via literal).
	_ = ModelPricing{
		GeminiSingleCachedPerMToken: 0.01,
		CodexReasoningPerMToken:     0.02,
	}
}

// --- Compute: known models ---

func TestCompute_Sonnet46_OutputOnly(t *testing.T) {
	// 1M output tokens at $15/M = $15.0000
	tokens := provider.TokenCounts{Output: 1_000_000}
	cost := Compute("claude-sonnet-4-6", tokens)
	if cost.Unknown {
		t.Fatal("expected known cost")
	}
	if cost.USD != 15.0 {
		t.Errorf("got %v, want 15.0", cost.USD)
	}
}

func TestCompute_Opus46_InputOnly(t *testing.T) {
	// 1M input tokens at $15/M = $15.0000
	tokens := provider.TokenCounts{Input: 1_000_000}
	cost := Compute("claude-opus-4-6", tokens)
	if cost.Unknown {
		t.Fatal("expected known cost")
	}
	if cost.USD != 15.0 {
		t.Errorf("got %v, want 15.0", cost.USD)
	}
}

func TestCompute_Haiku45_AllFourTypes(t *testing.T) {
	// 1M each: input=$0.80, cache_create=$1.00, cache_read=$0.08, output=$4.00
	// total = $5.88
	tokens := provider.TokenCounts{
		Input:         1_000_000,
		CacheCreation: 1_000_000,
		CacheRead:     1_000_000,
		Output:        1_000_000,
	}
	cost := Compute("claude-haiku-4-5", tokens)
	if cost.Unknown {
		t.Fatal("expected known cost")
	}
	if cost.USD != 5.88 {
		t.Errorf("got %v, want 5.88", cost.USD)
	}
}

func TestCompute_Sonnet35_CacheReadRate(t *testing.T) {
	// 1M cache-read tokens at $0.30/M = $0.3000
	tokens := provider.TokenCounts{CacheRead: 1_000_000}
	cost := Compute("claude-3-5-sonnet-20241022", tokens)
	if cost.Unknown {
		t.Fatal("expected known cost")
	}
	if cost.USD != 0.3 {
		t.Errorf("got %v, want 0.3", cost.USD)
	}
}

func TestCompute_Haiku35_CacheCreationRate(t *testing.T) {
	// 1M cache-creation tokens at $1.00/M = $1.0000
	tokens := provider.TokenCounts{CacheCreation: 1_000_000}
	cost := Compute("claude-3-5-haiku-20241022", tokens)
	if cost.Unknown {
		t.Fatal("expected known cost")
	}
	if cost.USD != 1.0 {
		t.Errorf("got %v, want 1.0", cost.USD)
	}
}

func TestCompute_RoundingTo4DecimalPlaces(t *testing.T) {
	// 1 input token for claude-sonnet-4-6: $3/M = $0.000003
	// rounded to 4 dp = $0.0000
	tokens := provider.TokenCounts{Input: 1}
	cost := Compute("claude-sonnet-4-6", tokens)
	if cost.Unknown {
		t.Fatal("expected known cost")
	}
	// 4 decimal places: value should equal math.Round(v*10000)/10000
	rounded := float64(int64(cost.USD*10000+0.5)) / 10000
	if cost.USD != rounded {
		t.Errorf("cost %v is not rounded to 4 decimal places", cost.USD)
	}
}

func TestCompute_GeminiCachedThoughtsToolRates(t *testing.T) {
	// gemini-2.5-flash rates:
	// input=0.30, output=2.50, cached=0.075, thoughts=2.50, tool=0.30
	tokens := provider.TokenCounts{
		Input:     1_000_000,
		Output:    1_000_000,
		CacheRead: 1_000_000,
		Thoughts:  1_000_000,
		Tool:      1_000_000,
	}
	cost := Compute("gemini-2.5-flash", tokens)
	if cost.Unknown {
		t.Fatal("expected known cost")
	}
	if cost.USD != 5.675 {
		t.Errorf("got %v, want 5.675", cost.USD)
	}
}

func TestCompute_CodexRates(t *testing.T) {
	// gpt-5-codex: input=1.25, cache-read=0.125, output=10.00, thoughts->output
	tokens := provider.TokenCounts{
		Input:     1_000_000,
		CacheRead: 1_000_000,
		Output:    1_000_000,
		Thoughts:  1_000_000,
	}
	cost := Compute("gpt-5-codex", tokens)
	if cost.Unknown {
		t.Fatal("expected known cost")
	}
	if cost.USD != 21.375 {
		t.Errorf("got %v, want 21.375", cost.USD)
	}
}

// --- Compute: unknown model ---

func TestCompute_UnknownModel_ReturnsUnknown(t *testing.T) {
	cost := Compute("gpt-4", provider.TokenCounts{Input: 1000, Output: 1000})
	if !cost.Unknown {
		t.Fatal("expected Unknown=true for unrecognized model")
	}
}

func TestCompute_EmptyModel_ReturnsUnknown(t *testing.T) {
	cost := Compute("", provider.TokenCounts{Input: 1000})
	if !cost.Unknown {
		t.Fatal("expected Unknown=true for empty model string")
	}
}

func TestCompute_UnknownModel_NotZeroUSD(t *testing.T) {
	// The Unknown flag — not zero USD — is the cost indicator for unknown models.
	cost := Compute("unknown-model-xyz", provider.TokenCounts{Input: 1_000_000})
	if !cost.Unknown {
		t.Fatal("expected Unknown=true")
	}
	// Confirm the indicator is Unknown, not just USD==0
	known := Compute("claude-sonnet-4-6", provider.TokenCounts{})
	if known.Unknown {
		t.Error("zero-token known model should not be Unknown")
	}
}

// --- Cost.Add ---

func TestCostAdd_TwoKnown(t *testing.T) {
	a := Cost{USD: 1.5}
	b := Cost{USD: 2.5}
	got := a.Add(b)
	if got.Unknown {
		t.Fatal("expected known result")
	}
	if got.USD != 4.0 {
		t.Errorf("got %v, want 4.0", got.USD)
	}
}

func TestCostAdd_OneUnknown_PropagatesUnknown(t *testing.T) {
	a := Cost{USD: 5.0}
	b := Cost{Unknown: true}
	if !a.Add(b).Unknown {
		t.Error("adding unknown should propagate Unknown")
	}
	if !b.Add(a).Unknown {
		t.Error("adding unknown should propagate Unknown (reversed order)")
	}
}

func TestCostAdd_BothUnknown(t *testing.T) {
	a := Cost{Unknown: true}
	b := Cost{Unknown: true}
	if !a.Add(b).Unknown {
		t.Error("sum of two unknowns should be Unknown")
	}
}

// --- CacheTierMinutes ---

func TestCacheTierMinutes(t *testing.T) {
	if CacheTierMinutes != 5 {
		t.Errorf("CacheTierMinutes = %d, want 5", CacheTierMinutes)
	}
}
