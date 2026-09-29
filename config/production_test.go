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

func TestLoadProductionConfigReadsCandooSMSSettings(t *testing.T) {
	t.Setenv("CANDOO_SMS_ENABLED", "true")
	t.Setenv("CANDOO_SMS_API_KEY", "candoo-key")
	t.Setenv("CANDOO_SMS_MESSAGE_TYPE", "2")
	t.Setenv("CANDOO_SMS_RETRY_COUNT", "4")
	t.Setenv("CANDOO_SMS_VALIDITY_PERIOD", "300")
	t.Setenv("CANDOO_SMS_MAX_REQUESTS_PER_SECOND", "7")
	t.Setenv("CANDOO_SMS_STATUS_MAP", "-1:pending,100:successful,200:unsuccessful")

	cfg := loadCandooSMSConfig()
	if !cfg.Enabled || cfg.APIKey != "candoo-key" || cfg.MessageType != 2 || cfg.RetryCount != 4 || cfg.ValidityPeriod != 300 || cfg.MaxRequestsPerSecond != 7 || cfg.StatusCodeMap["100"] != "successful" {
		t.Fatalf("Candoo config = %+v", cfg)
	}
}

func TestValidateProductionConfigRejectsInvalidEnabledCandooSettings(t *testing.T) {
	cfg := &ProductionConfig{CandooSMS: CandooSMSConfig{
		Enabled:              true,
		MessageType:          5,
		RetryCount:           11,
		ValidityPeriod:       172801,
		Timeout:              0,
		MaxRequestsPerSecond: 0,
		HTTPMaxAttempts:      0,
	}}
	err := ValidateProductionConfig(cfg)
	if err == nil {
		t.Fatal("invalid Candoo configuration unexpectedly passed validation")
	}
	for _, want := range []string{"CANDOO_SMS_API_KEY", "CANDOO_SMS_MESSAGE_TYPE", "CANDOO_SMS_RETRY_COUNT", "CANDOO_SMS_VALIDITY_PERIOD", "CANDOO_SMS_TIMEOUT", "CANDOO_SMS_MAX_REQUESTS_PER_SECOND", "CANDOO_SMS_HTTP_MAX_ATTEMPTS", "CANDOO_SMS_STATUS_MAP"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validation error %q does not include %q", err, want)
		}
	}
}

func TestValidateProductionConfigRejectsInvalidCandooStatusCodeMap(t *testing.T) {
	cfg := &ProductionConfig{CandooSMS: CandooSMSConfig{
		Enabled:              true,
		APIKey:               "candoo-key",
		MessageType:          0,
		RetryCount:           0,
		ValidityPeriod:       0,
		Timeout:              time.Second,
		MaxRequestsPerSecond: 1,
		HTTPMaxAttempts:      1,
		StatusCodeMap: map[string]string{
			"not-a-code": "delivered",
			"-1":         "pending",
		},
	}}
	err := ValidateProductionConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "CANDOO_SMS_STATUS_MAP") {
		t.Fatalf("validation error = %v, want invalid CANDOO_SMS_STATUS_MAP", err)
	}
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

func TestValidateExternalShortLinkConfig(t *testing.T) {
	valid := ExternalShortLinkConfig{
		Enabled:             true,
		BaseURL:             "https://links.example.com",
		APIToken:            strings.Repeat("x", 32),
		RequestTimeout:      30 * time.Second,
		MappingSyncInterval: time.Minute,
		ClickSyncInterval:   5 * time.Minute,
		MappingBatchSize:    500,
		ClickPageSize:       1000,
		MaxClickPagesPerRun: 100,
	}
	if errors := validateExternalShortLinkConfig(valid); len(errors) != 0 {
		t.Fatalf("valid external short-link config errors = %v", errors)
	}

	invalid := valid
	invalid.BaseURL = "http://links.example.com"
	invalid.APIToken = strings.Repeat("x", 31) + "!"
	errors := validateExternalShortLinkConfig(invalid)
	if len(errors) != 2 {
		t.Fatalf("invalid external short-link config errors = %v, want HTTPS and URL-safe token errors", errors)
	}
	if errors := validateExternalShortLinkConfig(ExternalShortLinkConfig{}); len(errors) != 0 {
		t.Fatalf("disabled external short-link config errors = %v", errors)
	}

	invalidOrigin := valid
	invalidOrigin.BaseURL = "https://links.example.com/api?token=leak"
	if errors := validateExternalShortLinkConfig(invalidOrigin); len(errors) != 1 || !strings.Contains(errors[0], "origin URL") {
		t.Fatalf("invalid origin errors = %v, want one origin URL error", errors)
	}
}
