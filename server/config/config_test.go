package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadBotConfigLoadsLLMProviderSettings(t *testing.T) {
	restoreEnv := clearLLMEnv(t)
	defer restoreEnv()

	t.Setenv("LLM_PROVIDER_BASE_URL", "https://llm.example.test/v1")
	t.Setenv("LLM_MODEL", "test-model")
	t.Setenv("LLM_API_KEY", "test-secret")
	t.Setenv("LLM_TIMEOUT_SECONDS", "9")
	t.Setenv("LLM_MAX_BODY_BYTES", "12345")
	t.Setenv("LLM_MAX_PROMPT_CHARS", "123")
	t.Setenv("LLM_MAX_CONTEXT_BLOCKS", "3")
	t.Setenv("LLM_MAX_CONTEXT_BYTES", "456")
	t.Setenv("LLM_MAX_RESPONSE_BYTES", "789")
	t.Setenv("LLM_MAX_OUTPUT_TOKENS", "321")
	t.Setenv("LLM_USER_RATE_PER_MINUTE", "2")
	t.Setenv("LLM_IP_RATE_PER_MINUTE", "4")
	t.Setenv("LLM_USER_CONCURRENCY", "1")
	t.Setenv("LLM_USER_DAILY_QUOTA", "6")

	origCfg := ServerCfg
	defer func() { ServerCfg = origCfg }()

	require.NoError(t, LoadBotConfig())
	require.Equal(t, "https://llm.example.test/v1", ServerCfg.LLMProviderBaseURL)
	require.Equal(t, "test-model", ServerCfg.LLMModel)
	require.Equal(t, "test-secret", ServerCfg.LLMAPIKey)
	require.Equal(t, 9*time.Second, ServerCfg.LLMTimeout())
	require.EqualValues(t, 12345, ServerCfg.LLMMaxBodyBytes)
	require.Equal(t, 123, ServerCfg.LLMMaxPromptChars)
	require.Equal(t, 3, ServerCfg.LLMMaxContextBlocks)
	require.EqualValues(t, 456, ServerCfg.LLMMaxContextBytes)
	require.EqualValues(t, 789, ServerCfg.LLMMaxResponseBytes)
	require.Equal(t, 321, ServerCfg.LLMMaxOutputTokens)
	require.Equal(t, 2, ServerCfg.LLMUserRatePerMinute)
	require.Equal(t, 4, ServerCfg.LLMIPRatePerMinute)
	require.Equal(t, 1, ServerCfg.LLMUserConcurrency)
	require.Equal(t, 6, ServerCfg.LLMUserDailyQuota)
}

func TestLoadBotConfigUsesLLMDefaults(t *testing.T) {
	restoreEnv := clearLLMEnv(t)
	defer restoreEnv()

	origCfg := ServerCfg
	defer func() { ServerCfg = origCfg }()

	require.NoError(t, LoadBotConfig())
	require.Equal(t, 30*time.Second, ServerCfg.LLMTimeout())
	require.EqualValues(t, 256<<10, ServerCfg.LLMMaxBodyBytes)
	require.Equal(t, 8000, ServerCfg.LLMMaxPromptChars)
	require.Equal(t, 8, ServerCfg.LLMMaxContextBlocks)
	require.EqualValues(t, 128<<10, ServerCfg.LLMMaxContextBytes)
	require.EqualValues(t, 256<<10, ServerCfg.LLMMaxResponseBytes)
	require.Equal(t, 1024, ServerCfg.LLMMaxOutputTokens)
	require.Equal(t, 10, ServerCfg.LLMUserRatePerMinute)
	require.Equal(t, 60, ServerCfg.LLMIPRatePerMinute)
	require.Equal(t, 2, ServerCfg.LLMUserConcurrency)
	require.Equal(t, 100, ServerCfg.LLMUserDailyQuota)
}

func clearLLMEnv(t *testing.T) func() {
	t.Helper()

	keys := []string{
		"LLM_PROVIDER_BASE_URL",
		"LLM_MODEL",
		"LLM_API_KEY",
		"LLM_TIMEOUT_SECONDS",
		"LLM_MAX_BODY_BYTES",
		"LLM_MAX_PROMPT_CHARS",
		"LLM_MAX_CONTEXT_BLOCKS",
		"LLM_MAX_CONTEXT_BYTES",
		"LLM_MAX_RESPONSE_BYTES",
		"LLM_MAX_OUTPUT_TOKENS",
		"LLM_USER_RATE_PER_MINUTE",
		"LLM_IP_RATE_PER_MINUTE",
		"LLM_USER_CONCURRENCY",
		"LLM_USER_DAILY_QUOTA",
	}

	type saved struct {
		value string
		ok    bool
	}
	orig := make(map[string]saved, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		orig[key] = saved{value: value, ok: ok}
		require.NoError(t, os.Unsetenv(key))
	}

	return func() {
		for _, key := range keys {
			if orig[key].ok {
				_ = os.Setenv(key, orig[key].value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}
}
