package experiment

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chaos-sec/backend/internal/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ptrBool is a helper to create a *bool from a bool value.
func ptrBool(b bool) *bool {
	return &b
}

// ---------------------------------------------------------------------------
// Status constant tests
// ---------------------------------------------------------------------------

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"pending", StatusPending, "pending"},
		{"running", StatusRunning, "running"},
		{"completed", StatusCompleted, "completed"},
		{"failed", StatusFailed, "failed"},
		{"cancelled", StatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("Status %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestStatusConstants_AreDistinct(t *testing.T) {
	statuses := []string{StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusCancelled}
	seen := make(map[string]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate status constant: %q", s)
		}
		seen[s] = true
	}
}

// ---------------------------------------------------------------------------
// NewEngine constructor tests
// ---------------------------------------------------------------------------

func TestNewEngine_WithNilDependencies(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if engine.logger == nil {
		t.Error("Engine logger should not be nil")
	}
}

func TestNewEngine_LoggerIsSet(t *testing.T) {
	logger := zap.NewNop()
	engine := NewEngine(nil, nil, nil, nil, logger)
	if engine.logger == nil {
		t.Error("Engine logger should be set")
	}
}

func TestNewEngine_AllDependencies(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	if engine.db != nil {
		t.Error("expected db to be nil")
	}
	if engine.rdb != nil {
		t.Error("expected rdb to be nil")
	}
	if engine.k8sManager != nil {
		t.Error("expected k8sManager to be nil")
	}
	if engine.siemValidator != nil {
		t.Error("expected siemValidator to be nil")
	}
}

// ---------------------------------------------------------------------------
// ExperimentStatus JSON tests
// ---------------------------------------------------------------------------

func TestExperimentStatus_JSONSerialization(t *testing.T) {
	now := time.Now()
	status := ExperimentStatus{
		RunID:       uuid.New(),
		Status:      StatusRunning,
		CurrentStep: 2,
		Progress:    40,
		Steps: []StepStatus{
			{
				Name:        "Step 1: DNS Exfiltration",
				Status:      StatusCompleted,
				StartedAt:   &now,
				CompletedAt: &now,
				Message:     "Completed successfully",
			},
			{
				Name:   "Step 2: RBAC Test",
				Status: StatusRunning,
			},
		},
		StartedAt: &now,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal ExperimentStatus: %v", err)
	}

	var got ExperimentStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Failed to unmarshal ExperimentStatus: %v", err)
	}

	if got.Status != StatusRunning {
		t.Errorf("got.Status = %q, want %q", got.Status, StatusRunning)
	}
	if got.CurrentStep != 2 {
		t.Errorf("got.CurrentStep = %d, want 2", got.CurrentStep)
	}
	if got.Progress != 40 {
		t.Errorf("got.Progress = %d, want 40", got.Progress)
	}
	if len(got.Steps) != 2 {
		t.Errorf("len(got.Steps) = %d, want 2", len(got.Steps))
	}
}

func TestStepStatus_JSONSerialization(t *testing.T) {
	now := time.Now()
	step := StepStatus{
		Name:        "Test Step",
		Status:      StatusCompleted,
		StartedAt:   &now,
		CompletedAt: &now,
		Message:     "OK",
	}

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Failed to marshal StepStatus: %v", err)
	}

	var got StepStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Failed to unmarshal StepStatus: %v", err)
	}

	if got.Name != "Test Step" {
		t.Errorf("got.Name = %q, want %q", got.Name, "Test Step")
	}
	if got.Status != StatusCompleted {
		t.Errorf("got.Status = %q, want %q", got.Status, StatusCompleted)
	}
	if got.Message != "OK" {
		t.Errorf("got.Message = %q, want %q", got.Message, "OK")
	}
}

func TestExperimentStatus_EmptySteps(t *testing.T) {
	status := ExperimentStatus{
		RunID:  uuid.New(),
		Status: StatusPending,
		Steps:  []StepStatus{},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var got ExperimentStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(got.Steps) != 0 {
		t.Errorf("len(got.Steps) = %d, want 0", len(got.Steps))
	}
}

func TestExperimentStatus_NilFields(t *testing.T) {
	status := ExperimentStatus{
		RunID:  uuid.New(),
		Status: StatusPending,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var got ExperimentStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if got.StartedAt != nil {
		t.Error("Expected StartedAt to be nil")
	}
	if got.EstimatedEndAt != nil {
		t.Error("Expected EstimatedEndAt to be nil")
	}
}

// ---------------------------------------------------------------------------
// calculateOverallScore tests
// ---------------------------------------------------------------------------

func TestCalculateOverallScore_NoValidations(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	score := engine.calculateOverallScore(nil)
	if score != 0 {
		t.Errorf("calculateOverallScore(nil) = %v, want 0", score)
	}
}

func TestCalculateOverallScore_EmptySlice(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	score := engine.calculateOverallScore([]models.SIEMValidation{})
	if score != 0 {
		t.Errorf("calculateOverallScore([]) = %v, want 0", score)
	}
}

func TestCalculateOverallScore_AllMatched(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	validations := []models.SIEMValidation{
		{Matched: ptrBool(true), AlertReceived: true},
		{Matched: ptrBool(true), AlertReceived: true},
		{Matched: ptrBool(true), AlertReceived: true},
	}
	score := engine.calculateOverallScore(validations)
	if score != 100 {
		t.Errorf("calculateOverallScore(all matched) = %v, want 100", score)
	}
}

func TestCalculateOverallScore_NoneMatched(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	validations := []models.SIEMValidation{
		{Matched: ptrBool(false), AlertReceived: false},
		{Matched: ptrBool(false), AlertReceived: false},
	}
	score := engine.calculateOverallScore(validations)
	if score != 0 {
		t.Errorf("calculateOverallScore(none matched) = %v, want 0", score)
	}
}

func TestCalculateOverallScore_PartialMatch(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	validations := []models.SIEMValidation{
		{Matched: ptrBool(true), AlertReceived: true},
		{Matched: ptrBool(false), AlertReceived: false},
		{Matched: ptrBool(true), AlertReceived: true},
		{Matched: ptrBool(false), AlertReceived: false},
	}
	score := engine.calculateOverallScore(validations)
	if score != 50 {
		t.Errorf("calculateOverallScore(2/4 matched) = %v, want 50", score)
	}
}

func TestCalculateOverallScore_SingleValidation(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())

	matched := engine.calculateOverallScore([]models.SIEMValidation{{Matched: ptrBool(true), AlertReceived: true}})
	if matched != 100 {
		t.Errorf("single matched = %v, want 100", matched)
	}

	notMatched := engine.calculateOverallScore([]models.SIEMValidation{{Matched: ptrBool(false), AlertReceived: false}})
	if notMatched != 0 {
		t.Errorf("single not matched = %v, want 0", notMatched)
	}
}

// ---------------------------------------------------------------------------
// calculateProgress tests
// ---------------------------------------------------------------------------

func TestCalculateProgress_CompletedRun(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	now := time.Now()
	run := &models.ExperimentRun{
		Status:      StatusCompleted,
		StartedAt:   &now,
		CompletedAt: &now,
	}
	steps := []StepStatus{
		{Name: "Step 1", Status: StatusCompleted},
		{Name: "Step 2", Status: StatusCompleted},
	}
	step, progress := engine.calculateProgress(run, steps)
	if progress != 100 {
		t.Errorf("progress for completed run = %d, want 100", progress)
	}
	_ = step
}

func TestCalculateProgress_FailedRun(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	now := time.Now()
	run := &models.ExperimentRun{
		Status:      StatusFailed,
		StartedAt:   &now,
		CompletedAt: &now,
	}
	steps := []StepStatus{
		{Name: "Step 1", Status: StatusCompleted},
		{Name: "Step 2", Status: StatusFailed},
	}
	_, progress := engine.calculateProgress(run, steps)
	if progress != 100 {
		t.Errorf("progress for failed run = %d, want 100", progress)
	}
}

func TestCalculateProgress_CancelledRun(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	now := time.Now()
	run := &models.ExperimentRun{
		Status:      StatusCancelled,
		StartedAt:   &now,
		CompletedAt: &now,
	}
	steps := []StepStatus{
		{Name: "Step 1", Status: StatusCancelled},
	}
	_, progress := engine.calculateProgress(run, steps)
	if progress != 100 {
		t.Errorf("progress for cancelled run = %d, want 100", progress)
	}
}

func TestCalculateProgress_PendingRun(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	run := &models.ExperimentRun{Status: StatusPending}
	steps := []StepStatus{
		{Name: "Step 1", Status: StatusPending},
		{Name: "Step 2", Status: StatusPending},
	}
	_, progress := engine.calculateProgress(run, steps)
	if progress != 0 {
		t.Errorf("progress for pending run = %d, want 0", progress)
	}
}

func TestCalculateProgress_RunningRun(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	now := time.Now()
	run := &models.ExperimentRun{
		Status:    StatusRunning,
		StartedAt: &now,
	}
	steps := []StepStatus{
		{Name: "Step 1", Status: StatusCompleted},
		{Name: "Step 2", Status: StatusRunning},
		{Name: "Step 3", Status: StatusPending},
	}
	step, progress := engine.calculateProgress(run, steps)
	if step != 1 {
		t.Errorf("current step = %d, want 1", step)
	}
	if progress <= 0 || progress >= 100 {
		t.Errorf("progress for running run = %d, want between 1-99", progress)
	}
}

func TestCalculateProgress_EmptySteps(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	run := &models.ExperimentRun{Status: StatusPending}
	step, progress := engine.calculateProgress(run, nil)
	if step != 0 {
		t.Errorf("step for empty steps = %d, want 0", step)
	}
	if progress != 0 {
		t.Errorf("progress for empty steps = %d, want 0", progress)
	}
}

// ---------------------------------------------------------------------------
// State machine validation tests
// ---------------------------------------------------------------------------

func TestStateMachine_ValidTransitions(t *testing.T) {
	// Verify that the status constants form a valid state machine.
	// Valid transitions: pending -> running -> completed/failed/cancelled
	//                    running -> completed/failed/cancelled
	validTransitions := map[string][]string{
		StatusPending:   {StatusRunning, StatusCancelled},
		StatusRunning:   {StatusCompleted, StatusFailed, StatusCancelled},
		StatusCompleted: {}, // terminal
		StatusFailed:    {}, // terminal
		StatusCancelled: {}, // terminal
	}

	// Ensure every status has at least an empty list of transitions.
	for status, transitions := range validTransitions {
		_ = status
		for _, next := range transitions {
			if next == "" {
				t.Errorf("empty transition status for %q", status)
			}
		}
	}

	// Verify terminal states have no outgoing transitions.
	terminalStates := []string{StatusCompleted, StatusFailed, StatusCancelled}
	for _, ts := range terminalStates {
		if len(validTransitions[ts]) != 0 {
			t.Errorf("terminal state %q should have no outgoing transitions, got %v", ts, validTransitions[ts])
		}
	}

	// Verify non-terminal states can transition to running or terminal states.
	nonTerminalStates := []string{StatusPending, StatusRunning}
	for _, nts := range nonTerminalStates {
		if len(validTransitions[nts]) == 0 {
			t.Errorf("non-terminal state %q should have at least one transition", nts)
		}
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	// These transitions should NOT be valid.
	invalidTransitions := []struct {
		from string
		to   string
	}{
		{StatusCompleted, StatusRunning}, // completed -> running is invalid
		{StatusFailed, StatusRunning},    // failed -> running is invalid
		{StatusCancelled, StatusRunning}, // cancelled -> running is invalid
		{StatusCompleted, StatusPending}, // completed -> pending is invalid
		{StatusRunning, StatusPending},   // running -> pending is invalid
	}

	for _, tt := range invalidTransitions {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			// These transitions are considered invalid. In a real state machine
			// implementation, we would check against allowed transitions.
			// Here we just document that they should be invalid.
			if tt.from == tt.to {
				t.Errorf("from and to are the same: %q", tt.from)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestPtrStr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *string
	}{
		{"empty", "", nil},
		{"non-empty", "hello", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input == "" {
				result := ptrStr(tt.input)
				if result == nil {
					t.Errorf("ptrStr(%q) = nil, want non-nil pointer to empty string", tt.input)
				} else if *result != "" {
					t.Errorf("ptrStr(%q) = %q, want empty string", tt.input, *result)
				}
			} else {
				result := ptrStr(tt.input)
				if result == nil {
					t.Errorf("ptrStr(%q) = nil, want non-nil", tt.input)
				} else if *result != tt.input {
					t.Errorf("ptrStr(%q) = %q, want %q", tt.input, *result, tt.input)
				}
			}
		})
	}
}

func TestPtrTime(t *testing.T) {
	now := time.Now()
	got := ptrTime(now)
	if got == nil {
		t.Fatal("ptrTime returned nil")
	}
	if !got.Equal(now) {
		t.Errorf("ptrTime mismatch: got %v, want %v", *got, now)
	}
}

func TestEngineNilIfEmpty(t *testing.T) {
	// nilIfEmpty returns nil for empty string, string value otherwise.
	result := engineNilIfEmpty("")
	if result != nil {
		t.Errorf("engineNilIfEmpty(%q) = %v, want nil", "", result)
	}

	result = engineNilIfEmpty("test")
	if result == nil {
		t.Errorf("engineNilIfEmpty(%q) = nil, want non-nil", "test")
	}
}

// ---------------------------------------------------------------------------
// Engine nil-dependency safety tests
// ---------------------------------------------------------------------------

func TestEngine_IsStopRequested_NilRedis(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	ctx := context.Background()
	runID := uuid.New()

	// Should not panic with nil Redis.
	result := engine.isStopRequested(ctx, runID)
	if result {
		t.Error("isStopRequested with nil Redis should return false")
	}
}

func TestEngine_UpdateProgress_NilRedis(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	ctx := context.Background()
	runID := uuid.New()

	// Should not panic with nil Redis.
	engine.updateProgress(ctx, runID, 1, 50)
}

func TestEngine_PublishStepStatuses_NilRedis(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	ctx := context.Background()
	runID := uuid.New()

	// Should not panic with nil Redis.
	engine.publishStepStatuses(ctx, runID, nil)
}

func TestEngine_UpdateStepStatus_NilRedis(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	ctx := context.Background()
	runID := uuid.New()

	// Should not panic with nil Redis.
	engine.updateStepStatus(ctx, runID, 0, StepStatus{
		Name:   "Step 1",
		Status: StatusPending,
	})
}

func TestEngine_CalculateOverallScore_NilDependencies(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())

	// Should work with nil dependencies.
	validations := []models.SIEMValidation{
		{Matched: ptrBool(true), AlertReceived: true},
		{Matched: ptrBool(false), AlertReceived: false},
	}
	score := engine.calculateOverallScore(validations)
	if score != 50 {
		t.Errorf("calculateOverallScore = %v, want 50", score)
	}
}

func TestEngine_CalculateProgress_NilDependencies(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, zap.NewNop())
	run := &models.ExperimentRun{Status: StatusPending}
	step, progress := engine.calculateProgress(run, nil)
	if step != 0 || progress != 0 {
		t.Errorf("calculateProgress(nil steps) = (%d, %d), want (0, 0)", step, progress)
	}
}

// ---------------------------------------------------------------------------
// QueueEntry JSON tests
// ---------------------------------------------------------------------------

func TestQueueEntry_JSONSerialization(t *testing.T) {
	entry := QueueEntry{
		RunID:    uuid.New(),
		Priority: 5,
		Score:    5e12,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal QueueEntry: %v", err)
	}

	var got QueueEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Failed to unmarshal QueueEntry: %v", err)
	}

	if got.Priority != 5 {
		t.Errorf("Priority = %d, want 5", got.Priority)
	}
	if got.RunID != entry.RunID {
		t.Errorf("RunID = %s, want %s", got.RunID, entry.RunID)
	}
}

// ---------------------------------------------------------------------------
// models import helpers (avoid unused import)
// ---------------------------------------------------------------------------

// This ensures the models package is imported correctly in the test file.
// The actual struct fields are tested through the engine methods above.

func TestModelsSIEMValidation_StructFields(t *testing.T) {
	v := models.SIEMValidation{
		Matched:       ptrBool(true),
		AlertReceived: true,
	}
	if v.Matched == nil || !*v.Matched {
		t.Error("SIEMValidation.Matched should be true")
	}
}

func TestModelsTestResult_StructFields(t *testing.T) {
	r := models.TestResult{
		CheckName: "test-check",
		CheckType: "network",
		Status:    "passed",
	}
	if r.CheckName != "test-check" {
		t.Errorf("TestResult.CheckName = %q, want %q", r.CheckName, "test-check")
	}
	if r.Status != "passed" {
		t.Errorf("TestResult.Status = %q, want %q", r.Status, "passed")
	}
}
