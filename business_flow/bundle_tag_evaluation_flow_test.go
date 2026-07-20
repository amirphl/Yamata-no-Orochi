package businessflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
)

func TestExecutionConfigurationUsesRunSnapshot(t *testing.T) {
	reasoningEffort := "medium"
	temperature := 0.25
	proxy := "http://user:password@proxy.example.test:8080"
	original := config.SmartTagEvaluationConfig{
		PersonaAnalysis: config.SmartTagPromptConfig{SystemPrompt: "persona-v1"},
		TagScoring:      config.SmartTagPromptConfig{SystemPrompt: "scoring-v1"},
		OpenAI: config.SmartTagOpenAIConfig{
			BaseURL:         "https://v1.example.test",
			Model:           "model-v1",
			ReasoningEffort: &reasoningEffort,
			MaxOutputTokens: 1234,
			Temperature:     &temperature,
			Timeout:         45 * time.Second,
			MaxRetries:      2,
			HTTPProxy:       &proxy,
		},
		Batching: config.SmartTagBatchingConfig{TagBatchSize: 10},
		Validation: config.SmartTagValidationConfig{
			RequireExactTagCount: true,
			RequireExactTagIDs:   true,
		},
	}
	queuedFlow := &BundleTagEvaluationFlowImpl{cfg: original}
	run := &models.BundleTagEvaluationRun{
		PersonaAnalysisPromptSnapshot: original.PersonaAnalysis.SystemPrompt,
		ConfigurationSnapshot:         queuedFlow.mustMarshalJSON(queuedFlow.configurationSnapshot()),
	}

	current := original
	current.PersonaAnalysis.SystemPrompt = "persona-v2"
	current.TagScoring.SystemPrompt = "scoring-v2"
	current.OpenAI.Model = "model-v2"
	currentProxy := "http://new-proxy.example.test:8080"
	current.OpenAI.HTTPProxy = &currentProxy
	current.Validation.RequireExactTagIDs = false

	executionFlow := &BundleTagEvaluationFlowImpl{cfg: current}
	got, err := executionFlow.executionConfiguration(run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PersonaAnalysis.SystemPrompt != "persona-v1" || got.TagScoring.SystemPrompt != "scoring-v1" {
		t.Fatalf("expected snapshotted prompts, got persona=%q scoring=%q", got.PersonaAnalysis.SystemPrompt, got.TagScoring.SystemPrompt)
	}
	if got.OpenAI.Model != "model-v1" || got.Validation.RequireExactTagIDs != true {
		t.Fatalf("expected snapshotted execution settings, got model=%q exact_ids=%v", got.OpenAI.Model, got.Validation.RequireExactTagIDs)
	}
	if got.OpenAI.ReasoningEffort == nil || *got.OpenAI.ReasoningEffort != reasoningEffort ||
		got.OpenAI.Temperature == nil || *got.OpenAI.Temperature != temperature {
		t.Fatalf("expected snapshotted optional settings, got reasoning=%v temperature=%v", got.OpenAI.ReasoningEffort, got.OpenAI.Temperature)
	}
	if got.OpenAI.HTTPProxy == nil || *got.OpenAI.HTTPProxy != currentProxy {
		t.Fatalf("expected current operational proxy, got %v", got.OpenAI.HTTPProxy)
	}
}

func TestExecutionConfigurationSnapshotPreservesUnsetOptionalParameters(t *testing.T) {
	queuedFlow := &BundleTagEvaluationFlowImpl{cfg: config.SmartTagEvaluationConfig{
		PersonaAnalysis: config.SmartTagPromptConfig{SystemPrompt: "persona"},
		TagScoring:      config.SmartTagPromptConfig{SystemPrompt: "scoring"},
		OpenAI: config.SmartTagOpenAIConfig{
			Model:           "model-v1",
			MaxOutputTokens: 100,
			Timeout:         time.Second,
		},
	}}
	run := &models.BundleTagEvaluationRun{
		PersonaAnalysisPromptSnapshot: queuedFlow.cfg.PersonaAnalysis.SystemPrompt,
		ConfigurationSnapshot:         queuedFlow.mustMarshalJSON(queuedFlow.configurationSnapshot()),
	}

	reasoningEffort := "high"
	temperature := 0.5
	current := queuedFlow.cfg
	current.OpenAI.ReasoningEffort = &reasoningEffort
	current.OpenAI.Temperature = &temperature

	got, err := (&BundleTagEvaluationFlowImpl{cfg: current}).executionConfiguration(run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.OpenAI.ReasoningEffort != nil || got.OpenAI.Temperature != nil {
		t.Fatalf("expected unset snapshotted options to remain nil, got reasoning=%v temperature=%v", got.OpenAI.ReasoningEffort, got.OpenAI.Temperature)
	}
}

func TestBuildOpenAIPayloadOptionalParameters(t *testing.T) {
	flow := &BundleTagEvaluationFlowImpl{}
	baseConfig := config.SmartTagEvaluationConfig{
		PersonaAnalysis: config.SmartTagPromptConfig{SystemPrompt: "persona"},
		TagScoring:      config.SmartTagPromptConfig{SystemPrompt: "scoring"},
		OpenAI: config.SmartTagOpenAIConfig{
			Model:           "test-model",
			MaxOutputTokens: 100,
		},
	}

	assertOptionalParameters := func(t *testing.T, payload map[string]any, wantPresent bool) {
		t.Helper()
		_, hasTemperature := payload["temperature"]
		_, hasReasoning := payload["reasoning"]
		if hasTemperature != wantPresent || hasReasoning != wantPresent {
			t.Fatalf("optional parameter presence: temperature=%v reasoning=%v, want %v; payload=%v", hasTemperature, hasReasoning, wantPresent, payload)
		}
	}

	t.Run("omits unset parameters", func(t *testing.T) {
		assertOptionalParameters(t, flow.buildPersonaAnalysisPayload(baseConfig, "target"), false)
		assertOptionalParameters(t, flow.buildTagScoringPayload(baseConfig, "analysis", nil), false)
	})

	t.Run("includes explicitly configured parameters", func(t *testing.T) {
		reasoningEffort := "low"
		temperature := 0.0
		configured := baseConfig
		configured.OpenAI.ReasoningEffort = &reasoningEffort
		configured.OpenAI.Temperature = &temperature

		personaPayload := flow.buildPersonaAnalysisPayload(configured, "target")
		scoringPayload := flow.buildTagScoringPayload(configured, "analysis", nil)
		assertOptionalParameters(t, personaPayload, true)
		assertOptionalParameters(t, scoringPayload, true)
		if got := personaPayload["temperature"]; got != 0.0 {
			t.Fatalf("expected explicit zero temperature, got %v", got)
		}
	})
}

func TestNormalizeOpenAIResultHandlesNilClientResult(t *testing.T) {
	flow := &BundleTagEvaluationFlowImpl{}
	result, err := flow.normalizeOpenAIResult(nil, nil, map[string]any{"model": "test-model"}, "test-model", time.Now().UTC())
	if err == nil {
		t.Fatal("expected nil client result to become an error")
	}
	if result == nil || len(result.RequestPayload) == 0 || result.ModelName != "test-model" {
		t.Fatalf("expected safe audit result, got %+v", result)
	}
}

func TestNonRetryableHTTPStatusSurvivesRestart(t *testing.T) {
	if err := nonRetryableHTTPStatusError(401, "invalid key"); err == nil {
		t.Fatal("expected a prior 401 attempt to remain terminal after restart")
	}
	if err := nonRetryableHTTPStatusError(429, "rate limited"); err != nil {
		t.Fatalf("expected a prior 429 attempt to remain retryable, got %v", err)
	}
}

func TestExtractOpenAIResponseText(t *testing.T) {
	t.Run("output_text", func(t *testing.T) {
		raw := `{"id":"resp_123","output_text":"hello world"}`
		got, err := extractOpenAIResponseText(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hello world" {
			t.Fatalf("expected output_text, got %q", got)
		}
	})

	t.Run("nested output content", func(t *testing.T) {
		raw := `{"output":[{"content":[{"type":"output_text","text":"[1,2,3]"}]}]}`
		got, err := extractOpenAIResponseText(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "[1,2,3]" {
			t.Fatalf("expected nested text, got %q", got)
		}
	})

	t.Run("skips reasoning items", func(t *testing.T) {
		raw := `{"output":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"not the final output"}]},{"type":"message","content":[{"type":"output_text","text":"final output"}]}]}`
		got, err := extractOpenAIResponseText(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "final output" {
			t.Fatalf("expected final message text, got %q", got)
		}
	})
}

func TestNormalizePersona(t *testing.T) {
	input := "  cafe\u0301\r\nshop  "
	got := normalizePersona(input)
	want := "caf\u00e9\nshop"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestParseAndValidateBatchResponse(t *testing.T) {
	flow := &BundleTagEvaluationFlowImpl{
		cfg: config.SmartTagEvaluationConfig{
			Validation: config.SmartTagValidationConfig{
				RequireExactTagCount: true,
				RequireExactTagIDs:   true,
			},
		},
	}

	tags := []models.BundleTagEvaluationTagSnapshot{
		{TagID: 11, TagName: "tag 11", TagDisplayTitle: "Tag 11", TagAudiencePersona: "persona 11", TagAudienceCount: 100},
		{TagID: 12, TagName: "tag 12", TagDisplayTitle: "Tag 12", TagAudiencePersona: "persona 12", TagAudienceCount: 200},
	}

	t.Run("accepts valid response", func(t *testing.T) {
		resultsJSON := []map[string]any{
			{
				"tag_id":           11,
				"bundle_fit_score": 91,
				"fit_level":        "very_strong",
				"relation_type":    "direct",
				"reason":           "foo",
			},
			{
				"tag_id":           12,
				"bundle_fit_score": 42,
				"fit_level":        "medium",
				"relation_type":    "indirect",
				"reason":           "bar",
			},
		}
		body, _ := json.Marshal(resultsJSON)
		raw := `{"output_text":` + string(mustJSON(t, string(body))) + `}`

		results, rawResults, err := flow.parseAndValidateBatchResponse(raw, tags, flow.cfg.Validation)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 || len(rawResults) != 2 {
			t.Fatalf("expected two results, got results=%d raw=%d", len(results), len(rawResults))
		}
		if results[11].BundleFitScore == nil || *results[11].BundleFitScore != 91 {
			t.Fatalf("unexpected score for tag 11: %+v", results[11])
		}
	})

	t.Run("accepts Responses API scores envelope and campaign score alias", func(t *testing.T) {
		modelOutput := `{"scores":[{"tag_id":18,"campaign_fit_score":5,"fit_level":"unrelated","relation_type":"unrelated","reason":"reason"}]}`
		response := map[string]any{
			"id": "resp_123",
			"output": []any{
				map[string]any{
					"type":    "reasoning",
					"content": []any{},
				},
				map[string]any{
					"type": "message",
					"content": []any{
						map[string]any{
							"type": "output_text",
							"text": modelOutput,
						},
					},
				},
			},
		}
		raw := string(mustJSON(t, response))
		sampleTags := []models.BundleTagEvaluationTagSnapshot{{TagID: 18}}

		results, rawResults, err := flow.parseAndValidateBatchResponse(raw, sampleTags, flow.cfg.Validation)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 || results[18].BundleFitScore == nil || *results[18].BundleFitScore != 5 {
			t.Fatalf("unexpected parsed results: %+v", results)
		}
		if !strings.Contains(string(rawResults[18]), `"campaign_fit_score":5`) {
			t.Fatalf("expected original score payload to be preserved, got %s", rawResults[18])
		}
	})

	t.Run("rejects missing tag", func(t *testing.T) {
		raw := `{"output_text":"[{\"tag_id\":11,\"bundle_fit_score\":91,\"fit_level\":\"very_strong\",\"relation_type\":\"direct\",\"reason\":\"foo\"}]"}`
		if _, _, err := flow.parseAndValidateBatchResponse(raw, tags, flow.cfg.Validation); err == nil {
			t.Fatalf("expected validation error for missing tag")
		}
	})

	t.Run("rejects out of range score", func(t *testing.T) {
		raw := `{"output_text":"[{\"tag_id\":11,\"bundle_fit_score\":101,\"fit_level\":\"very_strong\",\"relation_type\":\"direct\",\"reason\":\"foo\"},{\"tag_id\":12,\"bundle_fit_score\":42,\"fit_level\":\"medium\",\"relation_type\":\"indirect\",\"reason\":\"bar\"}]"}`
		if _, _, err := flow.parseAndValidateBatchResponse(raw, tags, flow.cfg.Validation); err == nil {
			t.Fatalf("expected validation error for out-of-range score")
		}
	})

	t.Run("rejects missing score", func(t *testing.T) {
		raw := `{"output_text":"[{\"tag_id\":11,\"fit_level\":\"very_strong\",\"relation_type\":\"direct\",\"reason\":\"foo\"},{\"tag_id\":12,\"bundle_fit_score\":42,\"fit_level\":\"medium\",\"relation_type\":\"indirect\",\"reason\":\"bar\"}]"}`
		if _, _, err := flow.parseAndValidateBatchResponse(raw, tags, flow.cfg.Validation); err == nil {
			t.Fatalf("expected validation error for missing score")
		}
	})

	t.Run("rejects null score", func(t *testing.T) {
		raw := `{"output_text":"[{\"tag_id\":11,\"bundle_fit_score\":null,\"fit_level\":\"very_strong\",\"relation_type\":\"direct\",\"reason\":\"foo\"},{\"tag_id\":12,\"bundle_fit_score\":42,\"fit_level\":\"medium\",\"relation_type\":\"indirect\",\"reason\":\"bar\"}]"}`
		if _, _, err := flow.parseAndValidateBatchResponse(raw, tags, flow.cfg.Validation); err == nil {
			t.Fatalf("expected validation error for null score")
		}
	})
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return raw
}
