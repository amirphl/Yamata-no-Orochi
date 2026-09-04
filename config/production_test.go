package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSchedulerConfigReadsTagTestPerformanceSettings(t *testing.T) {
	t.Setenv("TAG_TEST_PERFORMANCE_SCHEDULER_ENABLED", "true")
	t.Setenv("TAG_TEST_PERFORMANCE_SCHEDULER_INTERVAL", "45s")
	t.Setenv("TAG_TEST_PERFORMANCE_SCHEDULER_BATCH_SIZE", "17")

	cfg := loadSchedulerConfig()
	if !cfg.TagTestPerformanceSchedulerEnabled {
		t.Fatal("TagTestPerformanceSchedulerEnabled = false, want true")
	}
	if cfg.TagTestPerformanceSchedulerInterval != 45*time.Second {
		t.Fatalf("TagTestPerformanceSchedulerInterval = %s, want 45s", cfg.TagTestPerformanceSchedulerInterval)
	}
	if cfg.TagTestPerformanceSchedulerBatchSize != 17 {
		t.Fatalf("TagTestPerformanceSchedulerBatchSize = %d, want 17", cfg.TagTestPerformanceSchedulerBatchSize)
	}
}

func TestReadConfigTextFilePreservesMultilineContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt")
	want := "first line\n\n  indented line\nlast line\n"
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatalf("write prompt fixture: %v", err)
	}

	got, err := readConfigTextFile(path, true)
	if err != nil {
		t.Fatalf("readConfigTextFile() error = %v", err)
	}
	if got != want {
		t.Fatalf("readConfigTextFile() = %q, want %q", got, want)
	}
}

func TestReadConfigTextFileAllowsMissingOptionalFile(t *testing.T) {
	got, err := readConfigTextFile(filepath.Join(t.TempDir(), "missing"), false)
	if err != nil {
		t.Fatalf("readConfigTextFile() error = %v", err)
	}
	if got != "" {
		t.Fatalf("readConfigTextFile() = %q, want empty string", got)
	}
}

func TestReadConfigTextFileRejectsMissingRequiredFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	_, err := readConfigTextFile(path, true)
	if err == nil {
		t.Fatal("readConfigTextFile() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("readConfigTextFile() error = %q, want path %q", err, path)
	}
}

func TestOptionalOpenAIEnvironmentValues(t *testing.T) {
	t.Run("unset values return nil", func(t *testing.T) {
		t.Setenv("TEST_OPTIONAL_STRING", "")
		t.Setenv("TEST_OPTIONAL_FLOAT", "")
		if got := getOptionalEnvString("TEST_OPTIONAL_STRING"); got != nil {
			t.Fatalf("getOptionalEnvString() = %q, want nil", *got)
		}
		if got := getOptionalEnvFloat64("TEST_OPTIONAL_FLOAT"); got != nil {
			t.Fatalf("getOptionalEnvFloat64() = %v, want nil", *got)
		}
	})

	t.Run("configured values return pointers", func(t *testing.T) {
		t.Setenv("TEST_OPTIONAL_STRING", "high")
		t.Setenv("TEST_OPTIONAL_FLOAT", "0")
		if got := getOptionalEnvString("TEST_OPTIONAL_STRING"); got == nil || *got != "high" {
			t.Fatalf("getOptionalEnvString() = %v, want high", got)
		}
		if got := getOptionalEnvFloat64("TEST_OPTIONAL_FLOAT"); got == nil || *got != 0 {
			t.Fatalf("getOptionalEnvFloat64() = %v, want 0", got)
		}
	})
}

func TestLoadSchedulerConfigReadsSmartTargetingCapacitySchedulerFlag(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv("SMART_TARGETING_CAPACITY_SCHEDULER_ENABLED", "")
		if loadSchedulerConfig().SmartTargetingCapacitySchedulerEnabled {
			t.Fatal("SmartTargetingCapacitySchedulerEnabled = true, want false")
		}
	})

	t.Run("enabled explicitly", func(t *testing.T) {
		t.Setenv("SMART_TARGETING_CAPACITY_SCHEDULER_ENABLED", "true")
		if !loadSchedulerConfig().SmartTargetingCapacitySchedulerEnabled {
			t.Fatal("SmartTargetingCapacitySchedulerEnabled = false, want true")
		}
	})
}

func TestLoadSchedulerConfigReadsIndependentSmartTargetingTestSamplingSchedulerFlag(t *testing.T) {
	t.Run("inherits capacity flag when unset", func(t *testing.T) {
		t.Setenv("SMART_TARGETING_CAPACITY_SCHEDULER_ENABLED", "true")
		t.Setenv("SMART_TARGETING_TEST_SAMPLING_SCHEDULER_ENABLED", "")
		if !loadSchedulerConfig().SmartTargetingTestSamplingSchedulerEnabled {
			t.Fatal("SmartTargetingTestSamplingSchedulerEnabled = false, want backward-compatible true")
		}
	})

	t.Run("sampling can run without capacity", func(t *testing.T) {
		t.Setenv("SMART_TARGETING_CAPACITY_SCHEDULER_ENABLED", "false")
		t.Setenv("SMART_TARGETING_TEST_SAMPLING_SCHEDULER_ENABLED", "true")
		cfg := loadSchedulerConfig()
		if cfg.SmartTargetingCapacitySchedulerEnabled || !cfg.SmartTargetingTestSamplingSchedulerEnabled {
			t.Fatalf("scheduler flags = capacity:%t sampling:%t, want false/true", cfg.SmartTargetingCapacitySchedulerEnabled, cfg.SmartTargetingTestSamplingSchedulerEnabled)
		}
	})

	t.Run("sampling can be disabled while capacity runs", func(t *testing.T) {
		t.Setenv("SMART_TARGETING_CAPACITY_SCHEDULER_ENABLED", "true")
		t.Setenv("SMART_TARGETING_TEST_SAMPLING_SCHEDULER_ENABLED", "false")
		cfg := loadSchedulerConfig()
		if !cfg.SmartTargetingCapacitySchedulerEnabled || cfg.SmartTargetingTestSamplingSchedulerEnabled {
			t.Fatalf("scheduler flags = capacity:%t sampling:%t, want true/false", cfg.SmartTargetingCapacitySchedulerEnabled, cfg.SmartTargetingTestSamplingSchedulerEnabled)
		}
	})
}

func TestLoadSchedulerConfigReadsMessageSendMockFlag(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv("CAMPAIGN_MESSAGE_SEND_MOCK_ENABLED", "")
		if loadSchedulerConfig().MessageSendMockEnabled {
			t.Fatal("MessageSendMockEnabled = true, want false")
		}
	})

	t.Run("enabled explicitly", func(t *testing.T) {
		t.Setenv("CAMPAIGN_MESSAGE_SEND_MOCK_ENABLED", "true")
		if !loadSchedulerConfig().MessageSendMockEnabled {
			t.Fatal("MessageSendMockEnabled = false, want true")
		}
	})
}

func TestValidateCryptoConfigAllowsDisabledCryptoWithoutProviderCredentials(t *testing.T) {
	if errors := validateCryptoConfig(CryptoConfig{Enabled: false}); len(errors) != 0 {
		t.Fatalf("validateCryptoConfig() errors = %v, want none", errors)
	}
}

func TestValidateCryptoConfigRequiresOxapayKeyWhenCryptoEnabled(t *testing.T) {
	errors := validateCryptoConfig(CryptoConfig{
		Enabled:         true,
		DefaultPlatform: "oxapay",
		Oxapay:          OxapayConfig{BaseURL: "https://api.oxapay.com"},
	})
	if len(errors) != 1 || !strings.Contains(errors[0], "OXA_API_KEY is required") {
		t.Fatalf("validateCryptoConfig() errors = %v, want missing OXA_API_KEY error", errors)
	}
}
