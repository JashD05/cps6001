package siem

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chaos-sec/backend/internal/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ExpectedAlert defines what alert we expect the SIEM to have produced
// after executing an attack step during an experiment run.
type ExpectedAlert struct {
	// AlertType is the category of alert expected (e.g., "privilege_escalation", "network_intrusion").
	AlertType string `json:"alert_type"`

	// Severity is the minimum expected severity level (e.g., "low", "medium", "high", "critical").
	Severity string `json:"severity"`

	// TimeWindowSeconds is the maximum number of seconds after the attack
	// within which the alert should appear in the SIEM.
	TimeWindowSeconds int `json:"time_window_seconds"`

	// Description is a human-readable explanation of what this alert represents.
	Description string `json:"description"`
}

// AlertCorrelation captures the result of matching a single expected alert
// against the alerts actually received from the SIEM.
type AlertCorrelation struct {
	// ExpectedAlert is the alert we were looking for.
	ExpectedAlert ExpectedAlert `json:"expected_alert"`

	// ReceivedAlert is the best-matching alert found in the SIEM, or nil if no match.
	ReceivedAlert *SIEMAlert `json:"received_alert,omitempty"`

	// Matched is true when a sufficiently similar alert was found in the SIEM.
	Matched bool `json:"matched"`

	// MatchDetails provides a human-readable explanation of why the match
	// succeeded or failed.
	MatchDetails string `json:"match_details"`
}

// ValidationResult aggregates the outcome of closed-loop SIEM validation
// for an entire experiment run.
type ValidationResult struct {
	// OverallStatus is one of "passed", "failed", or "partial".
	//   - passed  = every expected alert was detected
	//   - partial = some expected alerts were detected
	//   - failed  = no expected alerts were detected
	OverallStatus string `json:"overall_status"`

	// Score is a 0–100 numeric score representing the detection coverage.
	// 100 means every expected alert was matched; 0 means none.
	Score float64 `json:"score"`

	// Correlations holds the per-expected-alert correlation results.
	Correlations []AlertCorrelation `json:"correlations"`

	// Summary is a human-readable summary of the validation outcome.
	Summary string `json:"summary"`
}

// severityRank maps severity strings to numeric ranks so that we can
// compare whether a received alert meets the minimum expected severity.
var severityRank = map[string]int{
	"low":      1,
	"medium":   2,
	"high":     3,
	"critical": 4,
	"":         0,
}

// Validator performs closed-loop verification by comparing expected
// security alerts against the alerts actually produced by the SIEM after
// an attack step is executed.
type Validator struct {
	connector        SIEMConnector
	logger           *zap.Logger
	propagationDelay time.Duration
	alertQueryLimit  int
}

// ValidatorOption is a functional option for configuring a Validator.
type ValidatorOption func(*Validator)

// WithPropagationDelay sets the delay to wait before querying the SIEM
// to allow alerts to propagate through the pipeline.
func WithPropagationDelay(d time.Duration) ValidatorOption {
	return func(v *Validator) {
		v.propagationDelay = d
	}
}

// WithAlertQueryLimit sets the maximum number of alerts to request from
// the SIEM in a single query.
func WithAlertQueryLimit(n int) ValidatorOption {
	return func(v *Validator) {
		v.alertQueryLimit = n
	}
}

// WithValidatorLogger sets the structured logger for the validator.
func WithValidatorLogger(logger *zap.Logger) ValidatorOption {
	return func(v *Validator) {
		v.logger = logger.Named("siem_validator")
	}
}

// NewValidator creates a new Validator with the given SIEM connector
// and optional configuration.
func NewValidator(connector SIEMConnector, opts ...ValidatorOption) *Validator {
	v := &Validator{
		connector:        connector,
		logger:           zap.NewNop(),
		propagationDelay: 30 * time.Second,
		alertQueryLimit:  1000,
	}

	for _, opt := range opts {
		opt(v)
	}

	return v
}

// ValidateDetection is the main validation flow. It:
//  1. Waits for the alert propagation delay so the SIEM has time to ingest and correlate events.
//  2. Queries the SIEM for alerts within the experiment run's time window.
//  3. Correlates each expected alert against the received alerts.
//  4. Calculates an overall detection score.
//  5. Returns a detailed ValidationResult.
func (v *Validator) ValidateDetection(
	ctx context.Context,
	run *models.ExperimentRun,
	expectedAlerts []ExpectedAlert,
) (*ValidationResult, error) {
	if run == nil {
		return nil, fmt.Errorf("experiment run is required")
	}

	if len(expectedAlerts) == 0 {
		v.logger.Debug("no expected alerts defined, skipping validation",
			zap.String("run_id", run.ID.String()),
		)
		return &ValidationResult{
			OverallStatus: "passed",
			Score:         100,
			Summary:       "No expected alerts defined; validation passed by default.",
		}, nil
	}

	// Step 1: Wait for alert propagation delay.
	v.logger.Info("waiting for alert propagation",
		zap.String("run_id", run.ID.String()),
		zap.Duration("delay", v.propagationDelay),
	)

	select {
	case <-time.After(v.propagationDelay):
		// Propagation delay elapsed, continue.
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled while waiting for alert propagation: %w", ctx.Err())
	}

	// Step 2: Determine the time window for querying the SIEM.
	from := time.Now()
	if run.StartedAt != nil {
		from = *run.StartedAt
	}
	to := time.Now()
	if run.CompletedAt != nil {
		to = *run.CompletedAt
	}
	// Extend the 'to' time slightly to capture alerts that arrived just after
	// the run was marked complete.
	to = to.Add(5 * time.Second)

	alerts, err := v.querySIEMAlerts(ctx, run, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query SIEM alerts: %w", err)
	}

	v.logger.Info("SIEM alerts retrieved",
		zap.String("run_id", run.ID.String()),
		zap.Int("alert_count", len(alerts)),
	)

	// Step 3: Correlate each expected alert against received alerts.
	correlations := make([]AlertCorrelation, 0, len(expectedAlerts))
	for _, expected := range expectedAlerts {
		correlation, corrErr := v.correlateAlert(expected, alerts)
		if corrErr != nil {
			v.logger.Error("alert correlation failed",
				zap.String("expected_type", expected.AlertType),
				zap.Error(corrErr),
			)
			correlations = append(correlations, AlertCorrelation{
				ExpectedAlert: expected,
				Matched:       false,
				MatchDetails:  fmt.Sprintf("correlation error: %v", corrErr),
			})
			continue
		}
		correlations = append(correlations, *correlation)
	}

	// Step 4: Calculate overall score.
	score := v.calculateScore(correlations)

	// Step 5: Determine overall status.
	status := "failed"
	matchedCount := 0
	for _, c := range correlations {
		if c.Matched {
			matchedCount++
		}
	}

	if matchedCount == len(correlations) {
		status = "passed"
	} else if matchedCount > 0 {
		status = "partial"
	}

	summary := v.buildSummary(matchedCount, len(correlations), score, status)

	result := &ValidationResult{
		OverallStatus: status,
		Score:         score,
		Correlations:  correlations,
		Summary:       summary,
	}

	v.logger.Info("validation complete",
		zap.String("run_id", run.ID.String()),
		zap.String("status", result.OverallStatus),
		zap.Float64("score", result.Score),
		zap.Int("matched", matchedCount),
		zap.Int("total", len(correlations)),
	)

	return result, nil
}

// querySIEMAlerts queries the SIEM for all alerts associated with the
// experiment run within the given time window. It paginates through
// results if the initial query returns the maximum number of alerts.
func (v *Validator) querySIEMAlerts(
	ctx context.Context,
	run *models.ExperimentRun,
	from, to time.Time,
) ([]SIEMAlert, error) {
	var allAlerts []SIEMAlert
	offset := 0

	for {
		query := AlertQuery{
			TimeRange: TimeRange{
				From: from,
				To:   to,
			},
			ExperimentID: run.ExperimentID,
			RunID:        run.ID,
			Pagination: Pagination{
				Offset: offset,
				Limit:  v.alertQueryLimit,
			},
		}

		alerts, err := v.connector.QueryAlerts(ctx, query)
		if err != nil {
			return allAlerts, fmt.Errorf("SIEM query failed at offset %d: %w", offset, err)
		}

		allAlerts = append(allAlerts, alerts...)

		// If we received fewer results than the limit, we've reached the end.
		if len(alerts) < v.alertQueryLimit {
			break
		}

		offset += len(alerts)

		// Safety guard: don't fetch more than 10 pages to avoid infinite loops.
		if offset >= v.alertQueryLimit*10 {
			v.logger.Warn("stopping SIEM alert pagination after 10 pages",
				zap.Int("total_fetched", len(allAlerts)),
			)
			break
		}
	}

	return allAlerts, nil
}

// correlateAlert attempts to match a single expected alert against the
// received SIEM alerts. It uses type matching and optional severity
// comparison to determine if a corresponding alert was produced.
func (v *Validator) correlateAlert(
	expected ExpectedAlert,
	received []SIEMAlert,
) (*AlertCorrelation, error) {
	correlation := &AlertCorrelation{
		ExpectedAlert: expected,
		Matched:       false,
	}

	// Find the best matching alert by type (case-insensitive substring match)
	// and severity.
	var bestMatch *SIEMAlert
	var bestScore int

	for i := range received {
		alert := &received[i]
		matchScore := v.computeMatchScore(expected, alert)
		if matchScore > bestScore {
			bestScore = matchScore
			bestMatch = alert
		}
	}

	if bestMatch != nil && bestScore > 0 {
		correlation.Matched = true
		correlation.ReceivedAlert = bestMatch

		details := fmt.Sprintf(
			"Matched alert type %q (severity %s) at %s",
			bestMatch.Type,
			bestMatch.Severity,
			bestMatch.Timestamp.Format(time.RFC3339),
		)

		// Check severity meets expectation.
		expectedRank, ok := severityRank[strings.ToLower(expected.Severity)]
		if !ok {
			expectedRank = 0
		}
		receivedRank, ok := severityRank[strings.ToLower(bestMatch.Severity)]
		if !ok {
			receivedRank = 0
		}

		if expected.Severity != "" && receivedRank < expectedRank {
			correlation.Matched = false
			details += fmt.Sprintf(
				" — WARNING: received severity %q is below expected %q",
				bestMatch.Severity,
				expected.Severity,
			)
		}

		correlation.MatchDetails = details
	} else {
		correlation.MatchDetails = fmt.Sprintf(
			"No matching alert found for expected type %q (severity %q)",
			expected.AlertType,
			expected.Severity,
		)
	}

	return correlation, nil
}

// computeMatchScore returns a numeric score indicating how well an alert
// matches the expected alert criteria. A score of 0 means no match;
// higher scores indicate better matches.
func (v *Validator) computeMatchScore(expected ExpectedAlert, alert *SIEMAlert) int {
	score := 0
	expectedTypeLower := strings.ToLower(expected.AlertType)
	alertTypeLower := strings.ToLower(alert.Type)

	// Type matching: exact match gets the highest score, substring match gets partial.
	if alertTypeLower == expectedTypeLower {
		score += 100
	} else if strings.Contains(alertTypeLower, expectedTypeLower) || strings.Contains(expectedTypeLower, alertTypeLower) {
		score += 60
	} else {
		// No type match at all.
		return 0
	}

	// Severity bonus: if the received severity meets or exceeds the expected, add bonus.
	expectedRank, ok := severityRank[strings.ToLower(expected.Severity)]
	if !ok {
		expectedRank = 0
	}
	receivedRank, ok := severityRank[strings.ToLower(alert.Severity)]
	if !ok {
		receivedRank = 0
	}

	if expected.Severity != "" {
		if receivedRank >= expectedRank {
			score += 10
		}
		// Even if severity is lower than expected, we still have a type match.
	}

	return score
}

// calculateScore computes a 0–100 score from the correlation results.
// The score represents the percentage of expected alerts that were
// successfully detected, weighted by match quality.
func (v *Validator) calculateScore(correlations []AlertCorrelation) float64 {
	if len(correlations) == 0 {
		return 100
	}

	matchedCount := 0
	for _, c := range correlations {
		if c.Matched {
			matchedCount++
		}
	}

	// Simple percentage-based scoring.
	// Future iterations could weight by severity or partial matches.
	score := (float64(matchedCount) / float64(len(correlations))) * 100.0

	// Round to 2 decimal places.
	score = float64(int(score*100)) / 100

	return score
}

// buildSummary creates a human-readable summary of the validation result.
func (v *Validator) buildSummary(matched, total int, score float64, status string) string {
	switch status {
	case "passed":
		return fmt.Sprintf("All %d expected alerts were detected (score: %.1f/100).", total, score)
	case "partial":
		return fmt.Sprintf("%d of %d expected alerts were detected (score: %.1f/100). Some security controls may need tuning.", matched, total, score)
	case "failed":
		return fmt.Sprintf("None of the %d expected alerts were detected (score: %.1f/100). Security controls may not be functioning as expected.", total, score)
	default:
		return fmt.Sprintf("Validation result: %s (%d/%d matched, score: %.1f/100)", status, matched, total, score)
	}
}

// ExpectedAlertsFromValidation parses the SIEMValidation JSON field from
// an ExperimentTemplate to extract the list of expected alerts. This
// is a convenience function for the experiment engine.
func ExpectedAlertsFromValidation(siemValidationJSON json.RawMessage) ([]ExpectedAlert, error) {
	if len(siemValidationJSON) == 0 || string(siemValidationJSON) == "null" || string(siemValidationJSON) == "{}" {
		return nil, nil
	}

	// Try to unmarshal directly as []ExpectedAlert.
	var alerts []ExpectedAlert
	if err := json.Unmarshal(siemValidationJSON, &alerts); err == nil {
		return alerts, nil
	}

	// Fallback: try as a wrapper object with an "expected_alerts" key.
	var wrapper struct {
		ExpectedAlerts []ExpectedAlert `json:"expected_alerts"`
	}
	if err := json.Unmarshal(siemValidationJSON, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse SIEM validation config: %w", err)
	}

	return wrapper.ExpectedAlerts, nil
}

// ValidationResultToSIEMValidations converts a ValidationResult into
// SIEMValidation model records suitable for database persistence.
func ValidationResultToSIEMValidations(
	result *ValidationResult,
	runID uuid.UUID,
	attackPodID *uuid.UUID,
) []models.SIEMValidation {
	if result == nil {
		return nil
	}

	validations := make([]models.SIEMValidation, 0, len(result.Correlations))
	now := time.Now()

	for _, corr := range result.Correlations {
		matched := corr.Matched
		var receivedAt *time.Time
		var alertID *string
		matchDetailsJSON, _ := json.Marshal(corr.MatchDetails)
		if len(matchDetailsJSON) == 0 {
			matchDetailsJSON = json.RawMessage(`{}`)
		}

		siemRespJSON := json.RawMessage(`{}`)
		if corr.ReceivedAlert != nil {
			receivedAt = &corr.ReceivedAlert.Timestamp
			alertID = &corr.ReceivedAlert.ID
			respBytes, _ := json.Marshal(corr.ReceivedAlert)
			if len(respBytes) > 0 {
				siemRespJSON = respBytes
			}
		}

		severity := corr.ExpectedAlert.Severity

		v := models.SIEMValidation{
			RunID:                 runID,
			AttackPodID:           attackPodID,
			ExpectedAlertType:     corr.ExpectedAlert.AlertType,
			ExpectedAlertSeverity: &severity,
			AlertReceived:         matched,
			ReceivedAt:            receivedAt,
			SIEMResponse:          siemRespJSON,
			AlertID:               alertID,
			Matched:               &matched,
			MatchDetails:          matchDetailsJSON,
			ValidationStatus:      "pending",
			CheckedAt:             &now,
		}

		if matched {
			v.ValidationStatus = "detected"
		} else {
			v.ValidationStatus = "missed"
		}

		validations = append(validations, v)
	}

	return validations
}
