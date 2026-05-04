package experiment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/chaos-sec/backend/internal/kubernetes"
	"github.com/chaos-sec/backend/internal/models"
	"github.com/chaos-sec/backend/internal/siem"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Experiment run status constants representing the state machine.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// ExperimentStatus represents the current execution status and progress
// of an experiment run, including per-step detail.
type ExperimentStatus struct {
	RunID          uuid.UUID    `json:"run_id"`
	Status         string       `json:"status"`
	CurrentStep    int          `json:"current_step"`
	Progress       int          `json:"progress"` // 0-100
	Steps          []StepStatus `json:"steps"`
	StartedAt      *time.Time   `json:"started_at"`
	EstimatedEndAt *time.Time   `json:"estimated_end_at,omitempty"`
}

// StepStatus represents the execution status of a single template step
// within an experiment run.
type StepStatus struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Message     string     `json:"message,omitempty"`
}

// Engine is the core experiment execution engine. It orchestrates the
// full lifecycle of an experiment run including Kubernetes namespace
// creation, attacker pod deployment, attack execution, SIEM validation,
// result collection, and cleanup.
//
// The Engine uses the kubernetes.ClientManager to obtain cluster clients
// and then creates PodController and NamespaceManager instances for each
// execution to interact with the target cluster.
type Engine struct {
	db            *sql.DB
	rdb           *redis.Client
	k8sManager    *kubernetes.ClientManager
	siemValidator *siem.Validator
	logger        *zap.Logger
	// cancelFuncs tracks running experiments by runID, allowing StopExperiment
	// to cancel the context of a running experiment for prompt termination.
	cancelFuncs sync.Map // map[uuid.UUID]context.CancelFunc
	// Per-run cluster controllers (set by initForCluster before each run).
	clusterClient *kubernetes.ClusterClient
	podCtrl       *kubernetes.PodController
	nsMgr         *kubernetes.NamespaceManager
}

// NewEngine creates a new experiment Engine with the provided dependencies.
// The k8sManager can be nil for dry-run / development mode where K8s
// operations are skipped.
func NewEngine(
	db *sql.DB,
	rdb *redis.Client,
	k8sManager *kubernetes.ClientManager,
	siemValidator *siem.Validator,
	logger *zap.Logger,
) *Engine {
	return &Engine{
		db:            db,
		rdb:           rdb,
		k8sManager:    k8sManager,
		siemValidator: siemValidator,
		logger:        logger.Named("experiment_engine"),
	}
}

// initForCluster initializes the per-cluster controllers for the given cluster ID.
// It retrieves a cached ClusterClient from the ClientManager, or creates one from
// the database record if not cached. Then it creates PodController and
// NamespaceManager instances from the cluster client.
func (e *Engine) initForCluster(ctx context.Context, clusterID uuid.UUID) error {
	// Try to get a cached client from the ClientManager.
	client, ok := e.k8sManager.GetClient(clusterID.String())
	if !ok {
		// Client not cached — load cluster details from DB and create client.
		cluster := &models.KubernetesCluster{}
		var desc, k8sVersion sql.NullString
		var nodeCount sql.NullInt64
		var lastConnected sql.NullTime

		err := e.db.QueryRowContext(ctx, `
			SELECT id, organization_id, name, description, api_endpoint,
			       ca_certificate, client_certificate, client_key,
			       default_namespace, status, kubernetes_version, node_count,
			       last_connected_at, created_at, updated_at
			FROM kubernetes_clusters
			WHERE id = $1
		`, clusterID).Scan(
			&cluster.ID, &cluster.OrganizationID, &cluster.Name, &desc,
			&cluster.APIEndpoint, &cluster.CACertificate, &cluster.ClientCertificate,
			&cluster.ClientKey, &cluster.DefaultNamespace, &cluster.Status,
			&k8sVersion, &nodeCount, &lastConnected,
			&cluster.CreatedAt, &cluster.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to load cluster %s from DB: %w", clusterID, err)
		}
		if desc.Valid {
			cluster.Description = desc.String
		}
		if k8sVersion.Valid {
			cluster.KubernetesVersion = &k8sVersion.String
		}
		if nodeCount.Valid {
			nc := int(nodeCount.Int64)
			cluster.NodeCount = &nc
		}
		if lastConnected.Valid {
			cluster.LastConnectedAt = &lastConnected.Time
		}

		client, err = e.k8sManager.RegisterClusterFromConfig(cluster)
		if err != nil {
			return fmt.Errorf("failed to connect to cluster %s: %w", clusterID, err)
		}
	}

	e.clusterClient = client

	podCtrl, err := kubernetes.NewPodController(client)
	if err != nil {
		return fmt.Errorf("failed to create pod controller: %w", err)
	}
	e.podCtrl = podCtrl

	nsMgr, err := kubernetes.NewNamespaceManager(client)
	if err != nil {
		return fmt.Errorf("failed to create namespace manager: %w", err)
	}
	e.nsMgr = nsMgr

	return nil
}

// ExecuteExperiment runs the full experiment execution flow:
//  1. Load experiment and templates from DB
//  2. Create experiment run record (if not already created)
//  3. For each template step:
//     a. Create isolated namespace
//     b. Deploy attacker pod
//     c. Wait for pod ready
//     d. Execute attack command
//     e. Collect results
//     f. Query SIEM for alerts
//     g. Validate alerts against expectations
//     h. Record test results
//     i. Clean up attacker pod
//  4. Calculate overall validation score
//  5. Update experiment run with final results
//  6. Send notifications
func (e *Engine) ExecuteExperiment(
	ctx context.Context,
	experimentID uuid.UUID,
	userID uuid.UUID,
) (*models.ExperimentRun, error) {
	e.logger.Info("starting experiment execution",
		zap.String("experiment_id", experimentID.String()),
		zap.String("user_id", userID.String()),
	)

	// Create a derived context that can be cancelled by StopExperiment.
	// This allows prompt termination of long-running operations like
	// WaitForPodReady or SIEM validation when a user requests a stop.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // Ensure context is cancelled when ExecuteExperiment returns

	// We need the runID to register the cancel func, so we load/create the run first,
	// then register. The cancel func is deregistered on exit via a deferred cleanup.
	var runID uuid.UUID
	defer func() {
		if runID != uuid.Nil {
			e.cancelFuncs.Delete(runID)
			e.logger.Debug("deregistered cancel func for experiment run",
				zap.String("run_id", runID.String()),
			)
		}
	}()

	// Step 1: Load experiment from DB.
	experiment, err := e.loadExperiment(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("failed to load experiment: %w", err)
	}

	// Step 2: Load experiment templates ordered by step index.
	templates, err := e.loadExperimentTemplates(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("failed to load experiment templates: %w", err)
	}

	if len(templates) == 0 {
		return nil, fmt.Errorf("experiment %s has no enabled templates", experimentID)
	}

	// Step 3: Find or create the experiment run record.
	run, err := e.findOrCreateRun(ctx, experiment, userID)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create experiment run: %w", err)
	}

	// Register the cancel func so StopExperiment can cancel this context.
	runID = run.ID
	e.cancelFuncs.Store(runID, cancel)
	e.logger.Info("registered cancel func for experiment run",
		zap.String("run_id", runID.String()),
	)

	// Check for stop signal before starting.
	if e.isStopRequested(ctx, run.ID) {
		e.updateRunStatus(ctx, run.ID, StatusCancelled, "Cancelled before execution started")
		return run, nil
	}

	// Update run status to running.
	now := time.Now()
	run.StartedAt = &now
	if err := e.updateRunStarted(ctx, run.ID, now); err != nil {
		e.logger.Error("failed to update run start time", zap.Error(err))
	}

	// Store initial step status in Redis for progress tracking.
	e.publishStepStatuses(ctx, run.ID, templates)

	// Resolve the cluster client and build per-cluster controllers.
	var podCtrl *kubernetes.PodController
	var nsMgr *kubernetes.NamespaceManager
	if e.k8sManager != nil {
		clusterID := run.ClusterID.String()
		client, ok := e.k8sManager.GetClient(clusterID)
		if !ok {
			// Try to find any connected cluster for this organisation.
			client, ok = e.findAnyConnectedCluster(ctx, experiment.OrganizationID)
			if !ok {
				e.logger.Warn("no connected Kubernetes cluster found, running in dry-run mode",
					zap.String("experiment_id", experimentID.String()),
				)
			}
		}
		if client != nil {
			podCtrl, _ = kubernetes.NewPodController(client)
			nsMgr, _ = kubernetes.NewNamespaceManager(client)
		}
	}

	// Track overall results.
	var allAttackPods []models.AttackPod
	var allTestResults []models.TestResult
	var allSIEMValidations []models.SIEMValidation
	totalSteps := len(templates)
	successfulSteps := 0
	failedSteps := 0

	siemValidator := e.siemValidator
	// In dry-run mode (no real Kubernetes client), skip SIEM validation so
	// experiments complete quickly instead of waiting on time-based checks.
	if podCtrl == nil && siemValidator != nil {
		e.logger.Info("dry-run mode detected; skipping SIEM validation for faster completion",
			zap.String("experiment_id", experimentID.String()),
		)
		siemValidator = nil
	}

	// Step 4: Execute each template step sequentially.
	for i, tmpl := range templates {
		stepName := fmt.Sprintf("Step %d: %s", tmpl.OrderIndex, e.templateName(ctx, tmpl))
		e.logger.Info("executing experiment step",
			zap.String("run_id", run.ID.String()),
			zap.Int("step", i+1),
			zap.Int("total_steps", totalSteps),
			zap.String("step_name", stepName),
		)

		// Update progress in Redis.
		progress := (i * 100) / totalSteps
		e.updateProgress(ctx, run.ID, i, progress)

		stepStart := time.Now()
		e.updateStepStatus(ctx, run.ID, i, StepStatus{
			Name:      stepName,
			Status:    StatusRunning,
			StartedAt: &stepStart,
		})

		// Check for stop signal.
		if e.isStopRequested(ctx, run.ID) {
			e.updateStepStatus(ctx, run.ID, i, StepStatus{
				Name:        stepName,
				Status:      StatusCancelled,
				StartedAt:   &stepStart,
				CompletedAt: ptrTime(time.Now()),
				Message:     "Cancelled by user",
			})
			e.cleanupAllSteps(ctx, podCtrl, nsMgr, allAttackPods)
			e.updateRunStatus(ctx, run.ID, StatusCancelled, "Cancelled by user during execution")
			return run, nil
		}

		attackPods, testResults, siemValidations, stepErr := e.executeStep(
			ctx, run, tmpl, podCtrl, nsMgr, siemValidator,
		)

		stepComplete := time.Now()
		stepStatus := StepStatus{
			Name:        stepName,
			StartedAt:   &stepStart,
			CompletedAt: &stepComplete,
		}

		if stepErr != nil {
			failedSteps++
			stepStatus.Status = StatusFailed
			stepStatus.Message = stepErr.Error()
			e.logger.Error("experiment step failed",
				zap.String("run_id", run.ID.String()),
				zap.Int("step", i+1),
				zap.Error(stepErr),
			)
		} else {
			successfulSteps++
			stepStatus.Status = StatusCompleted
			stepStatus.Message = "Step completed successfully"
		}

		e.updateStepStatus(ctx, run.ID, i, stepStatus)

		allAttackPods = append(allAttackPods, attackPods...)
		allTestResults = append(allTestResults, testResults...)
		allSIEMValidations = append(allSIEMValidations, siemValidations...)
	}

	// Update progress to 100%.
	e.updateProgress(ctx, run.ID, totalSteps-1, 100)

	// Step 5: Calculate overall validation score.
	overallScore := e.calculateOverallScore(allSIEMValidations)

	// Step 6: Build result summary.
	overallStatus := StatusCompleted
	if failedSteps == totalSteps {
		overallStatus = StatusFailed
	}

	resultSummary := e.buildResultSummary(
		totalSteps, successfulSteps, failedSteps,
		allAttackPods, allSIEMValidations, overallScore, overallStatus,
	)

	// Step 7: Update the experiment run with final results.
	completedAt := time.Now()
	var durationMs int64
	if run.StartedAt != nil {
		durationMs = completedAt.Sub(*run.StartedAt).Milliseconds()
	}

	if err := e.updateRunCompleted(ctx, run.ID, overallStatus, completedAt, durationMs, resultSummary); err != nil {
		e.logger.Error("failed to update experiment run completion", zap.Error(err))
	}

	// Persist SIEM validations to DB.
	if err := e.persistSIEMValidations(ctx, allSIEMValidations); err != nil {
		e.logger.Error("failed to persist SIEM validations", zap.Error(err))
	}

	// Persist test results to DB.
	if err := e.persistTestResults(ctx, allTestResults); err != nil {
		e.logger.Error("failed to persist test results", zap.Error(err))
	}

	// Step 8: Cleanup attacker pods and namespaces.
	if experiment.AutoCleanup {
		e.cleanupAllSteps(ctx, podCtrl, nsMgr, allAttackPods)
	}

	// Step 9: Send notifications.
	e.sendNotifications(ctx, run, resultSummary)

	e.logger.Info("experiment execution completed",
		zap.String("run_id", run.ID.String()),
		zap.String("status", overallStatus),
		zap.Int64("duration_ms", durationMs),
		zap.Float64("score", overallScore),
	)

	run.Status = overallStatus
	run.CompletedAt = &completedAt
	run.DurationMs = &durationMs
	run.ResultSummary = resultSummary

	return run, nil
}

// StopExperiment stops a running experiment by:
//  1. Cancelling the execution context (prompts immediate termination)
//  2. Signalling stop via Redis (fallback for goroutines checking isStopRequested)
//  3. Cleaning up K8s resources (attacker pods, namespaces)
//  4. Updating the run status to "cancelled"
func (e *Engine) StopExperiment(ctx context.Context, runID uuid.UUID) error {
	e.logger.Info("stopping experiment", zap.String("run_id", runID.String()))

	// Step 1: Cancel the running experiment's context for prompt termination.
	// This unblocks any long-running operations (WaitForPodReady, SIEM validation, etc.)
	// that are listening on ctx.Done().
	if cancelFunc, ok := e.cancelFuncs.LoadAndDelete(runID); ok {
		cancel := cancelFunc.(context.CancelFunc)
		cancel()
		e.logger.Info("cancelled execution context for experiment run",
			zap.String("run_id", runID.String()),
		)
	} else {
		e.logger.Debug("no running cancel func found for experiment run; may already be stopped or not yet started",
			zap.String("run_id", runID.String()),
		)
	}

	// Step 2: Signal the stop via Redis so the running ExecuteExperiment goroutine
	// can check it and exit gracefully (fallback mechanism).
	if e.rdb != nil {
		stopKey := fmt.Sprintf("experiment:stop:%s", runID.String())
		if err := e.rdb.Set(ctx, stopKey, "1", 30*time.Minute).Err(); err != nil {
			e.logger.Error("failed to set stop signal in Redis",
				zap.String("run_id", runID.String()),
				zap.Error(err),
			)
		}
	}

	// Step 3: Load the run to get its attack pods for cleanup.
	attackPods, err := e.loadAttackPods(ctx, runID)
	if err != nil {
		e.logger.Error("failed to load attack pods for cleanup",
			zap.String("run_id", runID.String()),
			zap.Error(err),
		)
	}

	// Step 4: Attempt K8s cleanup if a cluster is available.
	run, loadErr := e.loadRun(ctx, runID)
	if loadErr == nil && e.k8sManager != nil {
		clusterID := run.ClusterID.String()
		if client, ok := e.k8sManager.GetClient(clusterID); ok {
			podCtrl, _ := kubernetes.NewPodController(client)
			nsMgr, _ := kubernetes.NewNamespaceManager(client)
			e.cleanupAllSteps(ctx, podCtrl, nsMgr, attackPods)
		}
	}

	// Step 5: Update run status to cancelled.
	now := time.Now()
	if err := e.updateRunCancelled(ctx, runID, now); err != nil {
		return fmt.Errorf("failed to update run status to cancelled: %w", err)
	}

	e.logger.Info("experiment stopped", zap.String("run_id", runID.String()))
	return nil
}

// GetExperimentStatus returns the current execution status and progress
// of an experiment run by reading from both the database and Redis cache.
func (e *Engine) GetExperimentStatus(ctx context.Context, runID uuid.UUID) (*ExperimentStatus, error) {
	run, err := e.loadRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to load experiment run: %w", err)
	}

	status := &ExperimentStatus{
		RunID:     run.ID,
		Status:    run.Status,
		StartedAt: run.StartedAt,
	}

	// Try to load step statuses from Redis.
	steps, err := e.loadStepStatuses(ctx, runID)
	if err != nil {
		e.logger.Debug("could not load step statuses from Redis, building from DB",
			zap.Error(err),
		)
		steps = e.buildStepStatusesFromDB(ctx, run)
	}
	status.Steps = steps

	// Calculate current step and progress.
	status.CurrentStep, status.Progress = e.calculateProgress(run, steps)

	// Estimate end time if the run is still active.
	if run.Status == StatusRunning && run.StartedAt != nil {
		templates, tmplErr := e.loadExperimentTemplates(ctx, run.ExperimentID)
		if tmplErr == nil && len(templates) > 0 {
			totalDuration := 0
			for _, t := range templates {
				totalDuration += t.DurationSeconds
			}
			estimatedEnd := run.StartedAt.Add(time.Duration(totalDuration) * time.Second)
			status.EstimatedEndAt = &estimatedEnd
		}
	}

	return status, nil
}

// ---------------------------------------------------------------------------
// Step execution
// ---------------------------------------------------------------------------

// executeStep runs a single experiment template step. It returns the
// attack pods, test results, and SIEM validations created during the step,
// along with any error encountered.
func (e *Engine) executeStep(
	ctx context.Context,
	run *models.ExperimentRun,
	tmpl models.ExperimentTemplate,
	podCtrl *kubernetes.PodController,
	nsMgr *kubernetes.NamespaceManager,
	siemValidator *siem.Validator,
) ([]models.AttackPod, []models.TestResult, []models.SIEMValidation, error) {
	var attackPods []models.AttackPod
	var testResults []models.TestResult
	var siemValidations []models.SIEMValidation

	// Load the attack template.
	attackTmpl, err := e.loadAttackTemplate(ctx, tmpl.AttackTemplateID)
	if err != nil {
		return attackPods, testResults, siemValidations,
			fmt.Errorf("failed to load attack template: %w", err)
	}

	// a. Create isolated namespace.
	var nsName string
	if nsMgr != nil {
		nsName, err = nsMgr.CreateExperimentNamespace(ctx, "", run.ExperimentID.String())
		if err != nil {
			return attackPods, testResults, siemValidations,
				fmt.Errorf("failed to create namespace: %w", err)
		}
	} else {
		// Dry-run fallback: generate a plausible namespace name.
		nsName = fmt.Sprintf("chaos-sec-exp-%s", run.ID.String()[:8])
	}

	// b. Deploy attacker pod from the template's K8s manifest.
	var podName string
	var attackPodRecord models.AttackPod

	attackPodRecord = models.AttackPod{
		RunID:      run.ID,
		TemplateID: tmpl.AttackTemplateID,
		Namespace:  nsName,
		Status:     StatusPending,
	}
	podStart := time.Now()
	attackPodRecord.StartedAt = &podStart

	if podCtrl != nil {
		podConfig := e.buildAttackPodConfig(run, tmpl, attackTmpl, nsName)
		pod, deployErr := podCtrl.CreateAttackerPod(ctx, podConfig)
		if deployErr != nil {
			// Cleanup namespace on deployment failure.
			_ = e.nsMgr.DeleteNamespace(ctx, nsName)
			return attackPods, testResults, siemValidations,
				fmt.Errorf("failed to deploy attacker pod: %w", deployErr)
		}
		podName = pod.Name
		attackPodRecord.PodName = podName
		attackPodRecord.Status = StatusPending

		// Create the AttackPod DB record.
		attackPodRecord, err = e.createAttackPodRecord(ctx, attackPodRecord)
		if err != nil {
			e.logger.Error("failed to create attack pod record", zap.Error(err))
		}
		attackPods = append(attackPods, attackPodRecord)

		// c. Wait for pod to be ready.
		// Check for stop request before entering the potentially long WaitForPodReady call.
		if ctx.Err() != nil || e.isStopRequested(ctx, run.ID) {
			e.logger.Info("stop requested before waiting for pod ready, cancelling step",
				zap.String("run_id", run.ID.String()),
			)
			_ = podCtrl.ForceDeletePod(ctx, podName, nsName)
			_ = nsMgr.DeleteNamespace(ctx, nsName)
			attackPodRecord.Status = StatusCancelled
			terminatedAt := time.Now()
			attackPodRecord.TerminatedAt = &terminatedAt
			e.updateAttackPodRecord(ctx, attackPodRecord)
			return attackPods, testResults, siemValidations, fmt.Errorf("cancelled by user before waiting for pod ready")
		}
		waitErr := podCtrl.WaitForPodReady(ctx, podName, nsName,
			time.Duration(tmpl.DurationSeconds)*time.Second,
		)
		if waitErr != nil {
			testResults = append(testResults, models.TestResult{
				RunID:        run.ID,
				AttackPodID:  &attackPodRecord.ID,
				CheckName:    "pod_ready",
				CheckType:    "kubernetes",
				Status:       StatusFailed,
				Expected:     ptrStr("Running"),
				Actual:       ptrStr("Failed"),
				ErrorMessage: ptrStr(waitErr.Error()),
				Timestamp:    time.Now(),
			})

			// Cleanup and return.
			_ = podCtrl.ForceDeletePod(ctx, podName, nsName)
			_ = nsMgr.DeleteNamespace(ctx, nsName)

			attackPodRecord.Status = StatusFailed
			terminatedAt := time.Now()
			attackPodRecord.TerminatedAt = &terminatedAt
			e.updateAttackPodRecord(ctx, attackPodRecord)

			return attackPods, testResults, siemValidations,
				fmt.Errorf("pod %s/%s failed to become ready: %w", nsName, podName, waitErr)
		}

		// Update attack pod status to running.
		attackPodRecord.Status = StatusRunning

		// Retrieve pod IP and node.
		statusInfo, statusErr := podCtrl.GetPodStatus(ctx, podName, nsName)
		if statusErr == nil && statusInfo != nil {
			attackPodRecord.IPAddress = ptrStr(statusInfo.IP)
			attackPodRecord.NodeName = ptrStr(statusInfo.NodeName)
			attackPodRecord.Phase = ptrStr(statusInfo.Phase)
		}
		e.updateAttackPodRecord(ctx, attackPodRecord)

		testResults = append(testResults, models.TestResult{
			RunID:       run.ID,
			AttackPodID: &attackPodRecord.ID,
			CheckName:   "pod_ready",
			CheckType:   "kubernetes",
			Status:      StatusCompleted,
			Expected:    ptrStr("Running"),
			Actual:      ptrStr("Running"),
			Timestamp:   time.Now(),
		})

		// d. Execute attack command (if the manifest defines one).
		// Check for stop request before executing the attack.
		if ctx.Err() != nil || e.isStopRequested(ctx, run.ID) {
			e.logger.Info("stop requested before attack execution, cancelling step",
				zap.String("run_id", run.ID.String()),
			)
			_ = podCtrl.ForceDeletePod(ctx, podName, nsName)
			_ = nsMgr.DeleteNamespace(ctx, nsName)
			attackPodRecord.Status = StatusCancelled
			terminatedAt := time.Now()
			attackPodRecord.TerminatedAt = &terminatedAt
			e.updateAttackPodRecord(ctx, attackPodRecord)
			return attackPods, testResults, siemValidations, fmt.Errorf("cancelled by user before attack execution")
		}
		attackOutput, attackErr := e.executeAttack(ctx, podCtrl, podName, nsName, attackTmpl)
		if attackErr != nil {
			e.logger.Warn("attack execution produced an error",
				zap.String("pod", podName),
				zap.Error(attackErr),
			)
		}

		attackActual := "executed"
		if attackErr != nil {
			attackActual = fmt.Sprintf("error: %s", attackErr.Error())
		} else if attackOutput != "" {
			attackActual = attackOutput
		}
		testResults = append(testResults, models.TestResult{
			RunID:       run.ID,
			AttackPodID: &attackPodRecord.ID,
			CheckName:   "attack_execution",
			CheckType:   "attack",
			Status:      StatusCompleted,
			Expected:    ptrStr("attack_executed"),
			Actual:      &attackActual,
			Timestamp:   time.Now(),
		})

		// e. Collect results — capture pod logs.
		logs, logsErr := podCtrl.GetPodLogs(ctx, podName, nsName, nil)
		if logsErr != nil {
			e.logger.Warn("failed to collect pod logs",
				zap.String("pod", podName),
				zap.Error(logsErr),
			)
		} else if logs != "" {
			truncated := logs
			if len(truncated) > 10000 {
				truncated = truncated[:10000]
			}
			attackPodRecord.LogsSummary = &truncated
			e.updateAttackPodRecord(ctx, attackPodRecord)
		}

		// i. Clean up attacker pod (if immediate cleanup policy).
		if tmpl.CleanupPolicy == "immediate" || tmpl.CleanupPolicy == "" {
			_ = podCtrl.ForceDeletePod(ctx, podName, nsName)
			terminatedAt := time.Now()
			attackPodRecord.Status = StatusCompleted
			attackPodRecord.TerminatedAt = &terminatedAt
			e.updateAttackPodRecord(ctx, attackPodRecord)

			// Cleanup namespace.
			if nsMgr != nil {
				_ = nsMgr.DeleteNamespace(ctx, nsName)
			}
		}
	} else {
		// Dry-run mode: record a placeholder pod.
		podName = fmt.Sprintf("dry-run-%s-%d", run.ID.String()[:8], tmpl.OrderIndex)
		attackPodRecord.PodName = podName
		attackPodRecord.Status = StatusCompleted
		attackPodRecord, err = e.createAttackPodRecord(ctx, attackPodRecord)
		if err != nil {
			e.logger.Error("failed to create attack pod record (dry-run)", zap.Error(err))
		}
		attackPods = append(attackPods, attackPodRecord)

		testResults = append(testResults, models.TestResult{
			RunID:       run.ID,
			AttackPodID: &attackPodRecord.ID,
			CheckName:   "pod_ready",
			CheckType:   "kubernetes",
			Status:      StatusCompleted,
			Expected:    ptrStr("Running"),
			Actual:      ptrStr("Dry-run"),
			Timestamp:   time.Now(),
		})
	}

	// f. Query SIEM for alerts and g. Validate alerts against expectations.
	// Check for stop request before performing potentially long SIEM validation.
	if ctx.Err() != nil || e.isStopRequested(ctx, run.ID) {
		e.logger.Info("stop requested before SIEM validation, skipping",
			zap.String("run_id", run.ID.String()),
		)
		return attackPods, testResults, siemValidations, fmt.Errorf("cancelled by user before SIEM validation")
	}
	if siemValidator != nil && len(tmpl.SIEMValidation) > 0 {
		expectedAlerts, parseErr := siem.ExpectedAlertsFromValidation(tmpl.SIEMValidation)
		if parseErr != nil {
			e.logger.Error("failed to parse SIEM validation config",
				zap.Error(parseErr),
			)
		} else if len(expectedAlerts) > 0 {
			validationResult, validateErr := siemValidator.ValidateDetection(ctx, run, expectedAlerts)
			if validateErr != nil {
				e.logger.Error("SIEM validation failed",
					zap.String("run_id", run.ID.String()),
					zap.Error(validateErr),
				)
			} else {
				// Convert validation result to SIEMValidation model records.
				validations := siem.ValidationResultToSIEMValidations(
					validationResult, run.ID, &attackPodRecord.ID,
				)
				siemValidations = append(siemValidations, validations...)

				// Add a test result for the SIEM check.
				testResults = append(testResults, models.TestResult{
					RunID:       run.ID,
					AttackPodID: &attackPodRecord.ID,
					CheckName:   "siem_detection",
					CheckType:   "siem",
					Status:      validationResult.OverallStatus,
					Expected:    ptrStr("passed"),
					Actual:      ptrStr(validationResult.OverallStatus),
					Details:     json.RawMessage([]byte(validationResult.Summary)),
					Timestamp:   time.Now(),
				})
			}
		}
	}

	return attackPods, testResults, siemValidations, nil
}

// ---------------------------------------------------------------------------
// Attack pod configuration
// ---------------------------------------------------------------------------

// buildAttackPodConfig constructs an AttackPodConfig from the experiment
// template and attack template data.
func (e *Engine) buildAttackPodConfig(
	run *models.ExperimentRun,
	tmpl models.ExperimentTemplate,
	attackTmpl *models.AttackTemplate,
	namespace string,
) kubernetes.AttackPodConfig {
	// Parse the K8s manifest to extract image and command.
	var manifest struct {
		Spec struct {
			Containers []struct {
				Image   string   `json:"image"`
				Command []string `json:"command"`
			} `json:"containers"`
		} `json:"spec"`
	}

	image := "busybox:latest"
	var command []string

	if err := json.Unmarshal(attackTmpl.K8sManifest, &manifest); err == nil {
		if len(manifest.Spec.Containers) > 0 {
			if manifest.Spec.Containers[0].Image != "" {
				image = manifest.Spec.Containers[0].Image
			}
			if len(manifest.Spec.Containers[0].Command) > 0 {
				command = manifest.Spec.Containers[0].Command
			}
		}
	}

	// Parse parameters for additional env vars.
	var envVars map[string]string
	if len(attackTmpl.Parameters) > 0 {
		_ = json.Unmarshal(attackTmpl.Parameters, &envVars)
	}

	// Parse configuration for resource overrides.
	var limits kubernetes.ResourceLimits
	if len(tmpl.Configuration) > 0 {
		_ = json.Unmarshal(tmpl.Configuration, &limits)
	}

	return kubernetes.AttackPodConfig{
		ExperimentID:   run.ExperimentID.String(),
		RunID:          run.ID.String(),
		TemplateID:     tmpl.AttackTemplateID.String(),
		Namespace:      namespace,
		Image:          image,
		Command:        command,
		EnvVars:        envVars,
		ResourceLimits: &limits,
	}
}

// executeAttack runs the attack command within a pod using the PodController.
func (e *Engine) executeAttack(
	ctx context.Context,
	podCtrl *kubernetes.PodController,
	podName, namespace string,
	attackTmpl *models.AttackTemplate,
) (string, error) {
	// Parse the parameters for any attack command specification.
	var params map[string]interface{}
	if err := json.Unmarshal(attackTmpl.Parameters, &params); err != nil {
		e.logger.Debug("could not parse attack parameters as object",
			zap.Error(err),
		)
	}

	command, _ := params["command"].(string)
	if command == "" {
		e.logger.Info("no attack command specified, skipping exec",
			zap.String("template", attackTmpl.Slug),
		)
		return "no_command_specified", nil
	}

	e.logger.Info("executing attack command",
		zap.String("namespace", namespace),
		zap.String("pod_name", podName),
		zap.String("template", attackTmpl.Slug),
	)

	output, err := podCtrl.ExecuteInPod(ctx, podName, namespace, "attacker", command)
	if err != nil {
		return "", fmt.Errorf("attack command execution failed: %w", err)
	}

	return output, nil
}

// ---------------------------------------------------------------------------
// Database operations
// ---------------------------------------------------------------------------

// loadExperiment loads an experiment from the database by ID.
func (e *Engine) loadExperiment(ctx context.Context, experimentID uuid.UUID) (*models.Experiment, error) {
	var exp models.Experiment
	var scheduleCron *string
	err := e.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, description, status, created_by,
		       schedule_cron, auto_cleanup, notification_config, created_at, updated_at
		FROM experiments
		WHERE id = $1
	`, experimentID).Scan(
		&exp.ID, &exp.OrganizationID, &exp.Name, &exp.Description, &exp.Status,
		&exp.CreatedBy, &scheduleCron, &exp.AutoCleanup, &exp.NotificationConfig,
		&exp.CreatedAt, &exp.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query experiment %s: %w", experimentID, err)
	}
	exp.ScheduleCron = scheduleCron
	return &exp, nil
}

// loadExperimentTemplates loads all enabled templates for an experiment, ordered by step index.
func (e *Engine) loadExperimentTemplates(ctx context.Context, experimentID uuid.UUID) ([]models.ExperimentTemplate, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, experiment_id, attack_template_id, order_index, configuration,
		       target_namespaces, target_labels, duration_seconds, cleanup_policy,
		       siem_validation, enabled, created_at
		FROM experiment_templates
		WHERE experiment_id = $1 AND enabled = true
		ORDER BY order_index ASC
	`, experimentID)
	if err != nil {
		return nil, fmt.Errorf("query experiment templates: %w", err)
	}
	defer rows.Close()

	var templates []models.ExperimentTemplate
	for rows.Next() {
		var et models.ExperimentTemplate
		var targetNamespacesOut pq.StringArray
		if err := rows.Scan(
			&et.ID, &et.ExperimentID, &et.AttackTemplateID,
			&et.OrderIndex, &et.Configuration,
			&targetNamespacesOut, &et.TargetLabels,
			&et.DurationSeconds, &et.CleanupPolicy,
			&et.SIEMValidation, &et.Enabled, &et.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan experiment template: %w", err)
		}
		et.TargetNamespaces = []string(targetNamespacesOut)
		templates = append(templates, et)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate experiment templates: %w", err)
	}

	return templates, nil
}

// loadAttackTemplate loads a single attack template by ID.
func (e *Engine) loadAttackTemplate(ctx context.Context, templateID uuid.UUID) (*models.AttackTemplate, error) {
	var at models.AttackTemplate
	err := e.db.QueryRowContext(ctx, `
		SELECT id, name, slug, category, severity, description, mitre_attack_id,
		       k8s_manifest, parameters, prerequisites, expected_behavior, mitigation,
		       is_active, is_system, created_at, updated_at
		FROM attack_templates
		WHERE id = $1
	`, templateID).Scan(
		&at.ID, &at.Name, &at.Slug, &at.Category, &at.Severity,
		&at.Description, &at.MitreAttackID, &at.K8sManifest, &at.Parameters,
		&at.Prerequisites, &at.ExpectedBehavior, &at.Mitigation,
		&at.IsActive, &at.IsSystem, &at.CreatedAt, &at.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query attack template %s: %w", templateID, err)
	}
	return &at, nil
}

// findOrCreateRun finds the most recent pending/running run for this experiment,
// or creates a new one if none exists.
func (e *Engine) findOrCreateRun(ctx context.Context, exp *models.Experiment, userID uuid.UUID) (*models.ExperimentRun, error) {
	// Try to find an existing run created by the handler (status = pending or running).
	run, err := e.findActiveRun(ctx, exp.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to find active run: %w", err)
	}
	if run != nil {
		return run, nil
	}

	// Create a new run record.
	nextRunNumber := e.getNextRunNumber(ctx, exp.ID)

	// Resolve the cluster ID: find any connected cluster for this org, or fall back to the first available cluster.
	var clusterID uuid.UUID
	cluster, clusterOK := e.findAnyConnectedCluster(ctx, exp.OrganizationID)
	if clusterOK && cluster != nil {
		clusterID, _ = uuid.Parse(cluster.ClusterID())
	} else {
		// Fallback: pick any cluster from the database.
		var fallbackID uuid.UUID
		err := e.db.QueryRowContext(ctx, `SELECT id FROM kubernetes_clusters LIMIT 1`).Scan(&fallbackID)
		if err != nil {
			return nil, fmt.Errorf("no kubernetes cluster available for experiment run: %w", err)
		}
		clusterID = fallbackID
	}

	triggerType := "manual"
	triggeredBy := userID

	var newRun models.ExperimentRun
	var resultSummary sql.NullString
	err = e.db.QueryRowContext(ctx, `
		INSERT INTO experiment_runs (experiment_id, cluster_id, run_number, status, triggered_by, trigger_type)
		VALUES ($1, $2, $3, 'pending', $4, $5)
		RETURNING id, experiment_id, cluster_id, run_number, status, triggered_by, trigger_type,
		          started_at, completed_at, duration_ms, result_summary, error_message, cleanup_status, created_at
	`, exp.ID, clusterID, nextRunNumber, &triggeredBy, triggerType).Scan(
		&newRun.ID, &newRun.ExperimentID, &newRun.ClusterID, &newRun.RunNumber, &newRun.Status,
		&newRun.TriggeredBy, &newRun.TriggerType,
		&newRun.StartedAt, &newRun.CompletedAt, &newRun.DurationMs, &resultSummary,
		&newRun.ErrorMessage, &newRun.CleanupStatus, &newRun.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert experiment run: %w", err)
	}
	if resultSummary.Valid {
		newRun.ResultSummary = json.RawMessage(resultSummary.String)
	}

	return &newRun, nil
}

func (e *Engine) updateRunCancelled(ctx context.Context, runID uuid.UUID, cancelledAt time.Time) error {
	_, err := e.db.ExecContext(ctx, `
		UPDATE experiment_runs
		SET status = 'cancelled', completed_at = $1, error_message = 'Cancelled by user'
		WHERE id = $2
	`, cancelledAt, runID)
	return err
}

// findActiveRun finds the most recent pending or running run for an experiment.
func (e *Engine) findActiveRun(ctx context.Context, experimentID uuid.UUID) (*models.ExperimentRun, error) {
	var run models.ExperimentRun
	var resultSummary sql.NullString
	err := e.db.QueryRowContext(ctx, `
		SELECT id, experiment_id, cluster_id, run_number, status, triggered_by, trigger_type,
		       started_at, completed_at, duration_ms, result_summary, error_message, cleanup_status, created_at
		FROM experiment_runs
		WHERE experiment_id = $1 AND status IN ('pending', 'running')
		ORDER BY created_at DESC
		LIMIT 1
	`, experimentID).Scan(
		&run.ID, &run.ExperimentID, &run.ClusterID, &run.RunNumber, &run.Status,
		&run.TriggeredBy, &run.TriggerType,
		&run.StartedAt, &run.CompletedAt, &run.DurationMs, &resultSummary,
		&run.ErrorMessage, &run.CleanupStatus, &run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if resultSummary.Valid {
		run.ResultSummary = json.RawMessage(resultSummary.String)
	}
	return &run, nil
}

// getNextRunNumber returns the next run number for an experiment (max + 1, or 1 if none exist).
func (e *Engine) getNextRunNumber(ctx context.Context, experimentID uuid.UUID) int {
	var maxNum sql.NullInt64
	err := e.db.QueryRowContext(ctx, `
		SELECT MAX(run_number) FROM experiment_runs WHERE experiment_id = $1
	`, experimentID).Scan(&maxNum)
	if err != nil || !maxNum.Valid {
		return 1
	}
	return int(maxNum.Int64) + 1
}

// loadRun loads a full ExperimentRun from the database by ID.
func (e *Engine) loadRun(ctx context.Context, runID uuid.UUID) (*models.ExperimentRun, error) {
	var run models.ExperimentRun
	var resultSummary sql.NullString
	err := e.db.QueryRowContext(ctx, `
		SELECT id, experiment_id, cluster_id, run_number, status, triggered_by, trigger_type,
		       started_at, completed_at, duration_ms, result_summary, error_message, cleanup_status, created_at
		FROM experiment_runs
		WHERE id = $1
	`, runID).Scan(
		&run.ID, &run.ExperimentID, &run.ClusterID, &run.RunNumber, &run.Status,
		&run.TriggeredBy, &run.TriggerType,
		&run.StartedAt, &run.CompletedAt, &run.DurationMs, &resultSummary,
		&run.ErrorMessage, &run.CleanupStatus, &run.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query experiment run %s: %w", runID, err)
	}
	if resultSummary.Valid {
		run.ResultSummary = json.RawMessage(resultSummary.String)
	}
	return &run, nil
}

// loadAttackPods loads all AttackPod records for a given run.
func (e *Engine) loadAttackPods(ctx context.Context, runID uuid.UUID) ([]models.AttackPod, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, run_id, template_id, pod_name, namespace, node_name, ip_address,
		       status, phase, started_at, terminated_at, exit_code, logs_summary, created_at
		FROM attack_pods
		WHERE run_id = $1
		ORDER BY created_at ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("query attack pods for run %s: %w", runID, err)
	}
	defer rows.Close()

	var pods []models.AttackPod
	for rows.Next() {
		var p models.AttackPod
		if err := rows.Scan(
			&p.ID, &p.RunID, &p.TemplateID, &p.PodName, &p.Namespace,
			&p.NodeName, &p.IPAddress, &p.Status, &p.Phase,
			&p.StartedAt, &p.TerminatedAt, &p.ExitCode, &p.LogsSummary, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan attack pod: %w", err)
		}
		pods = append(pods, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attack pods: %w", err)
	}
	return pods, nil
}

// updateRunStarted updates the run status to 'running' and sets the started_at timestamp.
func (e *Engine) updateRunStarted(ctx context.Context, runID uuid.UUID, startedAt time.Time) error {
	_, err := e.db.ExecContext(ctx, `
		UPDATE experiment_runs
		SET status = 'running', started_at = $1
		WHERE id = $2
	`, startedAt, runID)
	return err
}

// updateRunCompleted updates the run with final status, completion time, duration, and result summary.
func (e *Engine) updateRunCompleted(ctx context.Context, runID uuid.UUID, status string, completedAt time.Time, durationMs int64, resultSummary json.RawMessage) error {
	_, err := e.db.ExecContext(ctx, `
		UPDATE experiment_runs
		SET status = $1, completed_at = $2, duration_ms = $3, result_summary = $4
		WHERE id = $5
	`, status, completedAt, durationMs, resultSummary, runID)
	return err
}

// updateRunStatus updates the run's status and optionally sets an error message.
func (e *Engine) updateRunStatus(ctx context.Context, runID uuid.UUID, status string, errorMsg string) error {
	_, err := e.db.ExecContext(ctx, `
		UPDATE experiment_runs
		SET status = $1, error_message = $2
		WHERE id = $3
	`, status, engineNilIfEmpty(errorMsg), runID)
	return err
}

// findAnyConnectedCluster finds any connected cluster for the given organization
// from the kubernetes_clusters table and returns a ClusterClient for it.
func (e *Engine) findAnyConnectedCluster(ctx context.Context, organizationID uuid.UUID) (*kubernetes.ClusterClient, bool) {
	var clusterID uuid.UUID
	err := e.db.QueryRowContext(ctx, `
		SELECT id FROM kubernetes_clusters
		WHERE organization_id = $1 AND status = 'connected'
		LIMIT 1
	`, organizationID).Scan(&clusterID)
	if err != nil {
		e.logger.Debug("no connected cluster found for organization",
			zap.String("organization_id", organizationID.String()),
			zap.Error(err),
		)
		return nil, false
	}

	if e.k8sManager != nil {
		client, ok := e.k8sManager.GetClient(clusterID.String())
		if ok {
			return client, true
		}
	}

	return nil, false
}

// createAttackPodRecord inserts an AttackPod record into the database.
func (e *Engine) createAttackPodRecord(ctx context.Context, pod models.AttackPod) (models.AttackPod, error) {
	err := e.db.QueryRowContext(ctx, `
		INSERT INTO attack_pods (run_id, template_id, pod_name, namespace, status, phase, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, pod.RunID, pod.TemplateID, pod.PodName, pod.Namespace,
		pod.Status, pod.Phase, pod.StartedAt,
	).Scan(&pod.ID, &pod.CreatedAt)
	return pod, err
}

// updateAttackPodRecord updates an existing AttackPod record.
func (e *Engine) updateAttackPodRecord(ctx context.Context, pod models.AttackPod) error {
	_, err := e.db.ExecContext(ctx, `
		UPDATE attack_pods
		SET status = $1, node_name = $2, ip_address = $3, phase = $4,
		    terminated_at = $5, exit_code = $6, logs_summary = $7
		WHERE id = $8
	`, pod.Status, pod.NodeName, pod.IPAddress, pod.Phase,
		pod.TerminatedAt, pod.ExitCode, pod.LogsSummary, pod.ID)
	return err
}

// persistSIEMValidations inserts SIEM validation records into the database.
func (e *Engine) persistSIEMValidations(ctx context.Context, validations []models.SIEMValidation) error {
	for _, v := range validations {
		_, err := e.db.ExecContext(ctx, `
			INSERT INTO siem_validations (
				run_id, attack_pod_id, expected_alert_type, expected_alert_severity,
				alert_received, received_at, siem_response, alert_id,
				matched, match_details, validation_status, checked_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, v.RunID, v.AttackPodID, v.ExpectedAlertType, v.ExpectedAlertSeverity,
			v.AlertReceived, v.ReceivedAt, v.SIEMResponse, v.AlertID,
			v.Matched, v.MatchDetails, v.ValidationStatus, v.CheckedAt)
		if err != nil {
			e.logger.Error("failed to persist SIEM validation",
				zap.String("run_id", v.RunID.String()),
				zap.Error(err),
			)
		}
	}
	return nil
}

// persistTestResults inserts test result records into the database.
func (e *Engine) persistTestResults(ctx context.Context, results []models.TestResult) error {
	for _, r := range results {
		_, err := e.db.ExecContext(ctx, `
			INSERT INTO test_results (
				run_id, attack_pod_id, check_name, check_type, status,
				expected, actual, details, error_message, timestamp
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, r.RunID, r.AttackPodID, r.CheckName, r.CheckType, r.Status,
			r.Expected, r.Actual, r.Details, r.ErrorMessage, r.Timestamp)
		if err != nil {
			e.logger.Error("failed to persist test result",
				zap.String("check_name", r.CheckName),
				zap.Error(err),
			)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Redis operations (progress tracking & stop signals)
// ---------------------------------------------------------------------------

// isStopRequested checks Redis for a stop signal for the given run.
func (e *Engine) isStopRequested(ctx context.Context, runID uuid.UUID) bool {
	if e.rdb == nil {
		return false
	}
	stopKey := fmt.Sprintf("experiment:stop:%s", runID.String())
	val, err := e.rdb.Get(ctx, stopKey).Result()
	if err != nil {
		return false
	}
	return val == "1"
}

// updateProgress stores the current execution progress in Redis.
func (e *Engine) updateProgress(ctx context.Context, runID uuid.UUID, currentStep, progress int) {
	if e.rdb == nil {
		return
	}
	key := fmt.Sprintf("experiment:progress:%s", runID.String())
	data := map[string]interface{}{
		"current_step": currentStep,
		"progress":     progress,
		"updated_at":   time.Now().Unix(),
	}
	if err := e.rdb.HSet(ctx, key, data).Err(); err != nil {
		e.logger.Debug("failed to update progress in Redis", zap.Error(err))
	}
	_ = e.rdb.Expire(ctx, key, 24*time.Hour).Err()
}

// publishStepStatuses stores the initial step list in Redis.
func (e *Engine) publishStepStatuses(ctx context.Context, runID uuid.UUID, templates []models.ExperimentTemplate) {
	if e.rdb == nil {
		return
	}
	key := fmt.Sprintf("experiment:steps:%s", runID.String())
	steps := make([]StepStatus, len(templates))
	for i, tmpl := range templates {
		steps[i] = StepStatus{
			Name:   fmt.Sprintf("Step %d: %s", tmpl.OrderIndex, e.templateName(ctx, tmpl)),
			Status: StatusPending,
		}
	}
	data, _ := json.Marshal(steps)
	_ = e.rdb.Set(ctx, key, data, 24*time.Hour).Err()
}

// updateStepStatus updates a single step's status in Redis.
func (e *Engine) updateStepStatus(ctx context.Context, runID uuid.UUID, stepIndex int, step StepStatus) {
	if e.rdb == nil {
		return
	}
	key := fmt.Sprintf("experiment:steps:%s", runID.String())
	data, err := e.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return
	}
	var steps []StepStatus
	if err := json.Unmarshal(data, &steps); err != nil {
		return
	}
	if stepIndex < len(steps) {
		steps[stepIndex] = step
	}
	updated, _ := json.Marshal(steps)
	_ = e.rdb.Set(ctx, key, updated, 24*time.Hour).Err()
}

// loadStepStatuses loads step statuses data from Redis.
func (e *Engine) loadStepStatuses(ctx context.Context, runID uuid.UUID) ([]StepStatus, error) {
	if e.rdb == nil {
		return nil, fmt.Errorf("redis not available")
	}
	key := fmt.Sprintf("experiment:steps:%s", runID.String())
	data, err := e.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var steps []StepStatus
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

// ---------------------------------------------------------------------------
// Scoring & result building
// ---------------------------------------------------------------------------

// calculateOverallScore computes a 0-100 score based on SIEM validation results.
func (e *Engine) calculateOverallScore(validations []models.SIEMValidation) float64 {
	if len(validations) == 0 {
		return 0
	}
	matched := 0
	for _, v := range validations {
		if v.AlertReceived {
			matched++
		}
	}
	return (float64(matched) / float64(len(validations))) * 100.0
}

// buildResultSummary creates the JSON result summary for the experiment run.
func (e *Engine) buildResultSummary(
	totalSteps, successfulSteps, failedSteps int,
	attackPods []models.AttackPod,
	siemValidations []models.SIEMValidation,
	overallScore float64,
	overallStatus string,
) json.RawMessage {
	totalPodsSpawned := len(attackPods)
	successfulAttacks := 0
	for _, p := range attackPods {
		if p.Status == StatusRunning || p.Status == StatusCompleted {
			successfulAttacks++
		}
	}

	siemExpected := len(siemValidations)
	siemReceived := 0
	for _, v := range siemValidations {
		if v.AlertReceived {
			siemReceived++
		}
	}

	var detectionRate float64
	if siemExpected > 0 {
		detectionRate = (float64(siemReceived) / float64(siemExpected)) * 100
	}

	findings := e.buildFindings(siemValidations, failedSteps)

	summary := models.RunResultSummary{
		TotalPodsSpawned:   totalPodsSpawned,
		SuccessfulAttacks:  successfulAttacks,
		BlockedAttacks:     totalPodsSpawned - successfulAttacks,
		SIEMAlertsExpected: siemExpected,
		SIEMAlertsReceived: siemReceived,
		DetectionRate:      detectionRate,
		OverallStatus:      overallStatus,
		Findings:           findings,
	}

	data, _ := json.Marshal(summary)
	return data
}

// buildFindings generates findings based on SIEM validation results.
func (e *Engine) buildFindings(validations []models.SIEMValidation, failedSteps int) []models.Finding {
	var findings []models.Finding

	for _, v := range validations {
		if !v.AlertReceived {
			findings = append(findings, models.Finding{
				Severity:       "high",
				Description:    fmt.Sprintf("SIEM did not detect expected alert type: %s", v.ExpectedAlertType),
				Recommendation: fmt.Sprintf("Review detection rules for %s alerts and ensure logging is configured correctly.", v.ExpectedAlertType),
			})
		}
	}

	if failedSteps > 0 {
		findings = append(findings, models.Finding{
			Severity:       "medium",
			Description:    fmt.Sprintf("%d experiment step(s) failed to execute", failedSteps),
			Recommendation: "Review attack template configurations and Kubernetes cluster capacity.",
		})
	}

	return findings
}

// ---------------------------------------------------------------------------
// Cleanup & notifications
// ---------------------------------------------------------------------------

// cleanupAllSteps cleans up all attack pods and their namespaces.
func (e *Engine) cleanupAllSteps(
	ctx context.Context,
	podCtrl *kubernetes.PodController,
	nsMgr *kubernetes.NamespaceManager,
	pods []models.AttackPod,
) {
	for _, p := range pods {
		if podCtrl != nil && p.Namespace != "" && p.PodName != "" {
			if err := podCtrl.ForceDeletePod(ctx, p.PodName, p.Namespace); err != nil {
				e.logger.Warn("failed to cleanup pod",
					zap.String("namespace", p.Namespace),
					zap.String("pod_name", p.PodName),
					zap.Error(err),
				)
			}
		}
		if nsMgr != nil && p.Namespace != "" {
			if err := nsMgr.DeleteNamespace(ctx, p.Namespace); err != nil {
				e.logger.Warn("failed to cleanup namespace",
					zap.String("namespace", p.Namespace),
					zap.Error(err),
				)
			}
		}
	}
}

// sendNotifications publishes a notification event to Redis for downstream
// consumers (e.g., email, Slack webhooks).
func (e *Engine) sendNotifications(ctx context.Context, run *models.ExperimentRun, summary json.RawMessage) {
	if e.rdb == nil {
		return
	}

	notificationData, _ := json.Marshal(map[string]interface{}{
		"run_id":         run.ID.String(),
		"experiment_id":  run.ExperimentID.String(),
		"status":         run.Status,
		"result_summary": summary,
		"timestamp":      time.Now(),
	})

	channel := "experiment:notifications"
	if err := e.rdb.Publish(ctx, channel, notificationData).Err(); err != nil {
		e.logger.Warn("failed to publish notification event",
			zap.String("run_id", run.ID.String()),
			zap.Error(err),
		)
	}
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// calculateProgress determines the current step index and percentage
// completion based on the run status and step data.
func (e *Engine) calculateProgress(run *models.ExperimentRun, steps []StepStatus) (int, int) {
	if run.Status == StatusCompleted || run.Status == StatusFailed || run.Status == StatusCancelled {
		return len(steps), 100
	}

	if run.Status == StatusPending {
		return 0, 0
	}

	completedSteps := 0
	for i, s := range steps {
		if s.Status == StatusCompleted || s.Status == StatusFailed {
			completedSteps = i + 1
		}
	}

	if len(steps) == 0 {
		return 0, 0
	}

	progress := (completedSteps * 100) / len(steps)
	return completedSteps, progress
}

// buildStepStatusesFromDB constructs step statuses information from database
// records when Redis data is unavailable.
func (e *Engine) buildStepStatusesFromDB(ctx context.Context, run *models.ExperimentRun) []StepStatus {
	attackPods, err := e.loadAttackPods(ctx, run.ID)
	if err != nil {
		return nil
	}

	steps := make([]StepStatus, len(attackPods))
	for i, pod := range attackPods {
		steps[i] = StepStatus{
			Name:        fmt.Sprintf("Attack: %s", pod.PodName),
			Status:      pod.Status,
			StartedAt:   pod.StartedAt,
			CompletedAt: pod.TerminatedAt,
		}
	}
	return steps
}

// templateName attempts to load the attack template name for a step.
func (e *Engine) templateName(ctx context.Context, tmpl models.ExperimentTemplate) string {
	attackTmpl, err := e.loadAttackTemplate(ctx, tmpl.AttackTemplateID)
	if err != nil {
		return fmt.Sprintf("template-%s", tmpl.AttackTemplateID.String()[:8])
	}
	return attackTmpl.Name
}

// ptrStr returns a pointer to the given string.
func ptrStr(s string) *string {
	return &s
}

// ptrTime returns a pointer to the given time.
func ptrTime(t time.Time) *time.Time {
	return &t
}

// engineNilIfEmpty returns nil for empty strings (useful for nullable DB columns).
// Named differently from nilIfEmpty in handlers.go to avoid redeclaration.
func engineNilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
