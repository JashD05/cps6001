package siem

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chaos-sec/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectedAlertsFromValidation_NilJSON(t *testing.T) {
	alerts, err := ExpectedAlertsFromValidation(nil)
	assert.NoError(t, err)
	assert.Nil(t, alerts)
}

func TestExpectedAlertsFromValidation_EmptyJSON(t *testing.T) {
	alerts, err := ExpectedAlertsFromValidation(json.RawMessage(""))
	assert.NoError(t, err)
	assert.Nil(t, alerts)
}

func TestExpectedAlertsFromValidation_NullJSON(t *testing.T) {
	alerts, err := ExpectedAlertsFromValidation(json.RawMessage("null"))
	assert.NoError(t, err)
	assert.Nil(t, alerts)
}

func TestExpectedAlertsFromValidation_EmptyObjectJSON(t *testing.T) {
	alerts, err := ExpectedAlertsFromValidation(json.RawMessage("{}"))
	assert.NoError(t, err)
	assert.Nil(t, alerts)
}

func TestExpectedAlertsFromValidation_ValidDirectArray(t *testing.T) {
	raw := json.RawMessage(`[
		{"alert_type": "network_flow", "severity": "high", "time_window_seconds": 300, "description": "Test alert"},
		{"alert_type": "privilege_escalation", "severity": "critical", "time_window_seconds": 120, "description": "Privilege test"}
	]`)
	alerts, err := ExpectedAlertsFromValidation(raw)
	require.NoError(t, err)
	require.Len(t, alerts, 2)

	assert.Equal(t, "network_flow", alerts[0].AlertType)
	assert.Equal(t, "high", alerts[0].Severity)
	assert.Equal(t, 300, alerts[0].TimeWindowSeconds)
	assert.Equal(t, "Test alert", alerts[0].Description)

	assert.Equal(t, "privilege_escalation", alerts[1].AlertType)
	assert.Equal(t, "critical", alerts[1].Severity)
	assert.Equal(t, 120, alerts[1].TimeWindowSeconds)
}

func TestExpectedAlertsFromValidation_WrapperObject(t *testing.T) {
	raw := json.RawMessage(`{
		"expected_alerts": [
			{"alert_type": "test_alert", "severity": "medium", "time_window_seconds": 60}
		]
	}`)
	alerts, err := ExpectedAlertsFromValidation(raw)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, "test_alert", alerts[0].AlertType)
}

func TestExpectedAlertsFromValidation_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{invalid json}`)
	alerts, err := ExpectedAlertsFromValidation(raw)
	assert.Error(t, err)
	assert.Nil(t, alerts)
}

func TestValidationResultToSIEMValidations_NilResult(t *testing.T) {
	validations := ValidationResultToSIEMValidations(nil, uuid.New(), nil)
	assert.Nil(t, validations)
}

func TestValidationResultToSIEMValidations_EmptyCorrelations(t *testing.T) {
	result := &ValidationResult{
		OverallStatus: "passed",
		Score:         100,
		Correlations:  []AlertCorrelation{},
	}
	runID := uuid.New()
	validations := ValidationResultToSIEMValidations(result, runID, nil)
	assert.Empty(t, validations)
}

func TestValidationResultToSIEMValidations_MatchedCorrelation(t *testing.T) {
	runID := uuid.New()
	podID := uuid.New()
	result := &ValidationResult{
		OverallStatus: "passed",
		Score:         100,
		Correlations: []AlertCorrelation{
			{
				ExpectedAlert: ExpectedAlert{
					AlertType:         "network_flow",
					Severity:          "high",
					TimeWindowSeconds: 300,
					Description:       "Test",
				},
				ReceivedAlert: &SIEMAlert{
					ID:        "alert-123",
					Type:      "network_flow",
					Severity:  "high",
					Timestamp: time.Now(),
				},
				Matched:      true,
				MatchDetails: "Matched alert type \"network_flow\"",
			},
		},
	}

	validations := ValidationResultToSIEMValidations(result, runID, &podID)
	require.Len(t, validations, 1)
	v := validations[0]
	assert.Equal(t, runID, v.RunID)
	assert.Equal(t, &podID, v.AttackPodID)
	assert.Equal(t, "network_flow", v.ExpectedAlertType)
	assert.True(t, v.AlertReceived)
	assert.True(t, *v.Matched)
	assert.Equal(t, "detected", v.ValidationStatus)
	assert.NotNil(t, v.ReceivedAt)
	assert.Equal(t, "alert-123", *v.AlertID)
}

func TestValidationResultToSIEMValidations_UnmatchedCorrelation(t *testing.T) {
	runID := uuid.New()
	result := &ValidationResult{
		OverallStatus: "failed",
		Score:         0,
		Correlations: []AlertCorrelation{
			{
				ExpectedAlert: ExpectedAlert{
					AlertType:         "secret_access",
					Severity:          "critical",
					TimeWindowSeconds: 180,
				},
				ReceivedAlert: nil,
				Matched:       false,
				MatchDetails:  "No matching alert found for expected type \"secret_access\"",
			},
		},
	}

	validations := ValidationResultToSIEMValidations(result, runID, nil)
	require.Len(t, validations, 1)
	v := validations[0]
	assert.False(t, v.AlertReceived)
	assert.False(t, *v.Matched)
	assert.Equal(t, "missed", v.ValidationStatus)
	assert.Nil(t, v.ReceivedAt)
}

func TestValidationResultToSIEMValidations_MultipleCorrelations(t *testing.T) {
	runID := uuid.New()
	result := &ValidationResult{
		OverallStatus: "partial",
		Score:         50,
		Correlations: []AlertCorrelation{
			{
				ExpectedAlert: ExpectedAlert{AlertType: "type_a", Severity: "high"},
				Matched:       true,
				ReceivedAlert: &SIEMAlert{ID: "a1", Type: "type_a", Severity: "high", Timestamp: time.Now()},
			},
			{
				ExpectedAlert: ExpectedAlert{AlertType: "type_b", Severity: "critical"},
				Matched:       false,
				MatchDetails:  "No match",
			},
			{
				ExpectedAlert: ExpectedAlert{AlertType: "type_c", Severity: "medium"},
				Matched:       true,
				ReceivedAlert: &SIEMAlert{ID: "c1", Type: "type_c", Severity: "medium", Timestamp: time.Now()},
			},
		},
	}

	validations := ValidationResultToSIEMValidations(result, runID, nil)
	require.Len(t, validations, 3)

	assert.True(t, validations[0].AlertReceived)
	assert.True(t, *validations[0].Matched)
	assert.False(t, validations[1].AlertReceived)
	assert.False(t, *validations[1].Matched)
	assert.True(t, validations[2].AlertReceived)
}

func TestNewValidator_DefaultValues(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	assert.NotNil(t, v)
	assert.Equal(t, 30*time.Second, v.propagationDelay)
	assert.Equal(t, 1000, v.alertQueryLimit)
}

func TestNewValidator_WithOptions(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(
		connector,
		WithPropagationDelay(1*time.Minute),
		WithAlertQueryLimit(500),
	)

	assert.Equal(t, 1*time.Minute, v.propagationDelay)
	assert.Equal(t, 500, v.alertQueryLimit)
}

func TestValidator_ValidateDetection_NilRun(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	result, err := v.ValidateDetection(nil, nil, []ExpectedAlert{{AlertType: "test"}})
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "experiment run is required")
}

func TestValidator_ValidateDetection_NoExpectedAlerts(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	run := &models.ExperimentRun{ID: uuid.New()}
	result, err := v.ValidateDetection(nil, run, []ExpectedAlert{})
	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "passed", result.OverallStatus)
	assert.Equal(t, 100.0, result.Score)
	assert.Contains(t, result.Summary, "No expected alerts")
}

func TestValidator_ValidateDetection_ContextCancelled(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(
		connector,
		WithPropagationDelay(10*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	run := &models.ExperimentRun{ID: uuid.New()}
	result, err := v.ValidateDetection(ctx, run, []ExpectedAlert{{AlertType: "test"}})
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "context cancelled")
}

func TestValidator_ValidateDetection_AllMatched(t *testing.T) {
	t.Skip("requires running mock SIEM server")

	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(
		connector,
		WithPropagationDelay(10*time.Millisecond),
	)

	now := time.Now()
	run := &models.ExperimentRun{
		ID:          uuid.New(),
		StartedAt:   &now,
		CompletedAt: &now,
	}

	expected := []ExpectedAlert{
		{AlertType: "network_flow", Severity: "high", TimeWindowSeconds: 300},
		{AlertType: "intrusion", Severity: "critical", TimeWindowSeconds: 180},
	}

	result, err := v.ValidateDetection(context.Background(), run, expected)
	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "partial", result.OverallStatus)
	assert.Len(t, result.Correlations, 2)
}

func TestValidator_CalculateScore(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	tests := []struct {
		name          string
		correlations  []AlertCorrelation
		expectedScore float64
	}{
		{
			name:          "all matched",
			correlations:  []AlertCorrelation{{Matched: true}, {Matched: true}, {Matched: true}},
			expectedScore: 100.0,
		},
		{
			name:          "none matched",
			correlations:  []AlertCorrelation{{Matched: false}, {Matched: false}},
			expectedScore: 0.0,
		},
		{
			name:          "partial matched - 2 of 3",
			correlations:  []AlertCorrelation{{Matched: true}, {Matched: true}, {Matched: false}},
			expectedScore: 66.66,
		},
		{
			name:          "empty correlations",
			correlations:  []AlertCorrelation{},
			expectedScore: 100.0,
		},
		{
			name:          "single matched",
			correlations:  []AlertCorrelation{{Matched: true}},
			expectedScore: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := v.calculateScore(tt.correlations)
			assert.Equal(t, tt.expectedScore, score)
		})
	}
}

func TestValidator_ComputeMatchScore(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	tests := []struct {
		name     string
		expected ExpectedAlert
		alert    SIEMAlert
		minScore int
	}{
		{
			name:     "exact type match",
			expected: ExpectedAlert{AlertType: "network_flow", Severity: "high"},
			alert:    SIEMAlert{Type: "network_flow", Severity: "high"},
			minScore: 100,
		},
		{
			name:     "substring match",
			expected: ExpectedAlert{AlertType: "flow", Severity: "high"},
			alert:    SIEMAlert{Type: "network_flow", Severity: "high"},
			minScore: 60,
		},
		{
			name:     "no type match",
			expected: ExpectedAlert{AlertType: "flow", Severity: "high"},
			alert:    SIEMAlert{Type: "something_else", Severity: "high"},
			minScore: 0,
		},
		{
			name:     "type match with severity bonus",
			expected: ExpectedAlert{AlertType: "network_flow", Severity: "high"},
			alert:    SIEMAlert{Type: "network_flow", Severity: "high"},
			minScore: 110,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := v.computeMatchScore(tt.expected, &tt.alert)
			assert.GreaterOrEqual(t, score, tt.minScore)
		})
	}
}

func TestBuildSummary(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	tests := []struct {
		status  string
		matched int
		total   int
		score   float64
		summary string
	}{
		{"passed", 3, 3, 100.0, "All 3 expected alerts were detected"},
		{"partial", 1, 3, 33.33, "1 of 3 expected alerts were detected"},
		{"failed", 0, 2, 0.0, "None of the 2 expected alerts were detected"},
	}

	for _, tt := range tests {
		summary := v.buildSummary(tt.matched, tt.total, tt.score, tt.status)
		assert.Contains(t, summary, tt.summary)
	}
}

func TestValidationResult_Struct(t *testing.T) {
	result := &ValidationResult{
		OverallStatus: "passed",
		Score:         85.5,
		Correlations: []AlertCorrelation{
			{
				ExpectedAlert: ExpectedAlert{AlertType: "test", Severity: "high"},
				Matched:       true,
				MatchDetails:  "Matched",
			},
		},
		Summary: "Test summary",
	}

	assert.Equal(t, "passed", result.OverallStatus)
	assert.Equal(t, 85.5, result.Score)
	assert.Len(t, result.Correlations, 1)
	assert.Equal(t, "Test summary", result.Summary)
}

func TestAlertCorrelation_Struct(t *testing.T) {
	correlation := AlertCorrelation{
		ExpectedAlert: ExpectedAlert{
			AlertType:         "network_flow",
			Severity:          "high",
			TimeWindowSeconds: 300,
			Description:       "Test",
		},
		ReceivedAlert: &SIEMAlert{
			ID:        "alert-1",
			Type:      "network_flow",
			Severity:  "high",
			Timestamp: time.Now(),
		},
		Matched:      true,
		MatchDetails: "Matched alert type",
	}

	assert.Equal(t, "network_flow", correlation.ExpectedAlert.AlertType)
	assert.True(t, correlation.Matched)
	assert.NotNil(t, correlation.ReceivedAlert)
	assert.Equal(t, "alert-1", correlation.ReceivedAlert.ID)
}

func TestAlertCorrelation_NilReceivedAlert(t *testing.T) {
	correlation := AlertCorrelation{
		ExpectedAlert: ExpectedAlert{AlertType: "test"},
		Matched:       false,
		MatchDetails:  "No match found",
	}

	assert.Nil(t, correlation.ReceivedAlert)
	assert.False(t, correlation.Matched)
}

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity string
		rank     int
	}{
		{"low", 1},
		{"medium", 2},
		{"high", 3},
		{"critical", 4},
		{"", 0},
		{"unknown", 0},
	}

	for _, tt := range tests {
		rank := severityRank[tt.severity]
		assert.Equal(t, tt.rank, rank)
	}
}

func TestExpectedAlert_Struct(t *testing.T) {
	alert := ExpectedAlert{
		AlertType:         "test_type",
		Severity:          "high",
		TimeWindowSeconds: 120,
		Description:       "Test alert",
	}

	assert.Equal(t, "test_type", alert.AlertType)
	assert.Equal(t, "high", alert.Severity)
	assert.Equal(t, 120, alert.TimeWindowSeconds)
	assert.Equal(t, "Test alert", alert.Description)
}

func TestValidator_CorrelateAlert_NoMatch(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	expected := ExpectedAlert{AlertType: "nonexistent", Severity: "high"}
	received := []SIEMAlert{
		{ID: "1", Type: "something_else", Severity: "high"},
	}

	correlation, err := v.correlateAlert(expected, received)
	assert.NoError(t, err)
	assert.False(t, correlation.Matched)
	assert.Nil(t, correlation.ReceivedAlert)
	assert.Contains(t, correlation.MatchDetails, "No matching alert found")
}

func TestValidator_CorrelateAlert_SeverityMismatch(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	expected := ExpectedAlert{AlertType: "test", Severity: "critical"}
	received := []SIEMAlert{
		{ID: "1", Type: "test", Severity: "low"}, // lower than expected
	}

	correlation, err := v.correlateAlert(expected, received)
	assert.NoError(t, err)
	assert.False(t, correlation.Matched) // severity too low
	assert.NotNil(t, correlation.ReceivedAlert)
	assert.Contains(t, correlation.MatchDetails, "WARNING")
}

func TestValidator_CorrelateAlert_SeverityMeetsExpectation(t *testing.T) {
	connector := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})
	v := NewValidator(connector)

	expected := ExpectedAlert{AlertType: "test", Severity: "medium"}
	received := []SIEMAlert{
		{ID: "1", Type: "test", Severity: "high"}, // meets or exceeds
	}

	correlation, err := v.correlateAlert(expected, received)
	assert.NoError(t, err)
	assert.True(t, correlation.Matched)
	assert.NotNil(t, correlation.ReceivedAlert)
	assert.Equal(t, "1", correlation.ReceivedAlert.ID)
}

func TestSIEMAlert_Timestamp(t *testing.T) {
	now := time.Now()
	alert := SIEMAlert{
		ID:        "test",
		Type:      "test",
		Severity:  "info",
		Timestamp: now,
	}

	assert.Equal(t, now, alert.Timestamp)
}

func TestTimeRange_Struct(t *testing.T) {
	from := time.Now().Add(-1 * time.Hour)
	to := time.Now()

	tr := TimeRange{
		From: from,
		To:   to,
	}

	assert.Equal(t, from, tr.From)
	assert.Equal(t, to, tr.To)
}

func TestAlertQuery_Struct(t *testing.T) {
	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-24 * time.Hour),
			To:   time.Now(),
		},
		AlertType:  "test_type",
		Severity:   "high",
		Source:     "test_source",
		Pagination: Pagination{Offset: 0, Limit: 100},
	}

	assert.Equal(t, "test_type", query.AlertType)
	assert.Equal(t, "high", query.Severity)
	assert.Equal(t, 100, query.Pagination.Limit)
}

func TestPagination_Struct(t *testing.T) {
	p := Pagination{
		Offset: 50,
		Limit:  25,
	}

	assert.Equal(t, 50, p.Offset)
	assert.Equal(t, 25, p.Limit)
}

func TestMockSIEM_ImplementsSIEMConnector(t *testing.T) {
	var connector SIEMConnector
	ms := &MockSIEM{}
	connector = ms // compile-time check that MockSIEM implements SIEMConnector
	assert.NotNil(t, connector)
}

func TestMockSIEM_SendAlert_NotConnected(t *testing.T) {
	ms := NewMockSIEM(SIEMConfig{
		Endpoint: "http://localhost:9999", // non-existent server
		Timeout:  1 * time.Second,
	})

	alert := SIEMAlert{
		ID:        "test",
		Type:      "test",
		Severity:  "high",
		Timestamp: time.Now(),
	}

	err := ms.SendAlert(context.Background(), alert)
	assert.Error(t, err)
}

func TestMockSIEM_QueryAlerts_NotConnected(t *testing.T) {
	ms := NewMockSIEM(SIEMConfig{
		Endpoint: "http://localhost:9999",
		Timeout:  1 * time.Second,
	})

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 100},
	}

	_, err := ms.QueryAlerts(context.Background(), query)
	assert.Error(t, err)
}

func TestMockSIEM_HealthCheck_NotConnected(t *testing.T) {
	ms := NewMockSIEM(SIEMConfig{
		Endpoint: "http://localhost:9999",
		Timeout:  1 * time.Second,
	})

	err := ms.HealthCheck(context.Background())
	assert.Error(t, err)
}

func TestMockSIEM_Close(t *testing.T) {
	ms := NewMockSIEM(SIEMConfig{Endpoint: "http://localhost"})

	err := ms.Close()
	assert.NoError(t, err)
}

func TestSIEMConfig_Struct(t *testing.T) {
	config := SIEMConfig{
		Endpoint:   "https://siem.example.com:8443",
		APIKey:     "test-key",
		Username:   "admin",
		Password:   "secret",
		Timeout:    30 * time.Second,
		MaxRetries: 3,
	}

	assert.Equal(t, "https://siem.example.com:8443", config.Endpoint)
	assert.Equal(t, "test-key", config.APIKey)
	assert.Equal(t, "admin", config.Username)
	assert.Equal(t, "secret", config.Password)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxRetries)
}

func TestSIEMConfig_DefaultTimeout(t *testing.T) {
	config := SIEMConfig{
		Endpoint: "https://siem.example.com",
	}

	assert.Zero(t, config.Timeout)
}
