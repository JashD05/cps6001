package experiment

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/chaos-sec/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
)

// ReportService handles generation of experiment reports in various formats.
type ReportService struct {
	db *sql.DB
}

// NewReportService creates a new ReportService.
func NewReportService(db *sql.DB) *ReportService {
	return &ReportService{db: db}
}

// ReportData holds all the data needed to generate a report.
type ReportData struct {
	Experiment   models.Experiment
	Runs         []models.ExperimentRun
	Summary      *models.RunResultSummary
	GeneratedAt  time.Time
	Organization string
}

// GeneratePDFReport generates a PDF report for an experiment.
func (s *ReportService) GeneratePDFReport(ctx context.Context, experimentID uuid.UUID) ([]byte, error) {
	data, err := s.FetchReportData(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch report data: %w", err)
	}

	return s.RenderPDF(data)
}

// FetchReportData fetches all data needed for a report from the database.
func (s *ReportService) FetchReportData(ctx context.Context, experimentID uuid.UUID) (*ReportData, error) {
	data := &ReportData{
		GeneratedAt: time.Now(),
	}

	// Fetch experiment
	var description, scheduleCron sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT name, description, status, schedule_cron, created_at
		FROM experiments WHERE id = $1
	`, experimentID).Scan(
		&data.Experiment.Name, &description, &data.Experiment.Status,
		&scheduleCron, &data.Experiment.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch experiment: %w", err)
	}
	if description.Valid {
		data.Experiment.Description = description.String
	}
	data.Experiment.ID = experimentID

	// Fetch runs
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_number, status, triggered_by, trigger_type,
		       started_at, completed_at, duration_ms, result_summary, error_message
		FROM experiment_runs WHERE experiment_id = $1 ORDER BY run_number DESC
	`, experimentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch runs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var run models.ExperimentRun
		var errMsg, resultJSON sql.NullString
		if err := rows.Scan(
			&run.ID, &run.RunNumber, &run.Status, &run.TriggeredBy, &run.TriggerType,
			&run.StartedAt, &run.CompletedAt, &run.DurationMs, &resultJSON, &errMsg,
		); err != nil {
			continue
		}
		if errMsg.Valid {
			run.ErrorMessage = &errMsg.String
		}
		data.Runs = append(data.Runs, run)
	}

	return data, nil
}

// RenderPDF generates a PDF document from report data.
func (s *ReportService) RenderPDF(data *ReportData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// Header
	s.renderHeader(pdf, data)

	// Experiment Summary
	s.renderExperimentSummary(pdf, data)

	// Runs Table
	s.renderRunsTable(pdf, data)

	// Summary (if available)
	if data.Summary != nil {
		s.renderSummarySection(pdf, data.Summary)
	}

	// Footer
	s.renderFooter(pdf)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return buf.Bytes(), nil
}

func (s *ReportService) renderHeader(pdf *gofpdf.Fpdf, data *ReportData) {
	pdf.SetFont("Arial", "B", 20)
	pdf.SetTextColor(26, 54, 93)
	pdf.CellFormat(0, 12, "Chaos-Sec Experiment Report", "", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(0, 6, fmt.Sprintf("Generated: %s", data.GeneratedAt.Format(time.RFC1123)), "", 1, "C", false, 0, "")
	pdf.Ln(5)

	// Separator line
	pdf.SetDrawColor(26, 54, 93)
	pdf.SetLineWidth(0.5)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(5)
}

func (s *ReportService) renderExperimentSummary(pdf *gofpdf.Fpdf, data *ReportData) {
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.CellFormat(0, 8, "Experiment Details", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(0, 0, 0)

	fields := []struct {
		label string
		value string
	}{
		{"Experiment Name", data.Experiment.Name},
		{"Experiment ID", data.Experiment.ID.String()},
		{"Status", data.Experiment.Status},
		{"Created", data.Experiment.CreatedAt.Format("2006-01-02 15:04:05")},
	}

	for _, f := range fields {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(45, 6, f.label, "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 6, f.value, "", 1, "L", false, 0, "")
	}

	if data.Experiment.Description != "" {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(45, 6, "Description", "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 5, data.Experiment.Description, "", "L", false)
	}

	pdf.Ln(5)
}

func (s *ReportService) renderRunsTable(pdf *gofpdf.Fpdf, data *ReportData) {
	if len(data.Runs) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.CellFormat(0, 8, "Experiment Runs", "", 1, "L", false, 0, "")

	// Table header
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(26, 54, 93)
	pdf.SetTextColor(255, 255, 255)

	headers := []string{"Run #", "Status", "Triggered By", "Started", "Duration", "Result"}
	widths := []float64{20, 25, 35, 45, 25, 40}

	for i, h := range headers {
		pdf.CellFormat(widths[i], 8, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Table rows
	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(245, 245, 245)

	for i, run := range data.Runs {
		fill := i%2 == 0

		duration := "N/A"
		if run.DurationMs != nil && *run.DurationMs > 0 {
			duration = fmt.Sprintf("%.1fs", float64(*run.DurationMs)/1000)
		}

		started := "N/A"
		if run.StartedAt != nil && !run.StartedAt.IsZero() {
			started = run.StartedAt.Format("2006-01-02 15:04")
		}

		triggeredBy := "N/A"
		if run.TriggeredBy != nil {
			triggeredBy = run.TriggeredBy.String()
		}

		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(widths[0], 7, fmt.Sprintf("Run %d", run.RunNumber), "1", 0, "C", fill, 0, "")
		pdf.CellFormat(widths[1], 7, run.Status, "1", 0, "C", fill, 0, "")
		pdf.CellFormat(widths[2], 7, triggeredBy, "1", 0, "C", fill, 0, "")
		pdf.CellFormat(widths[3], 7, started, "1", 0, "C", fill, 0, "")
		pdf.CellFormat(widths[4], 7, duration, "1", 0, "C", fill, 0, "")

		result := "N/A"
		if run.ErrorMessage != nil && *run.ErrorMessage != "" {
			result = "Error"
		} else if run.Status == "completed" {
			result = "Success"
		}
		pdf.CellFormat(widths[5], 7, result, "1", 1, "C", fill, 0, "")
	}

	pdf.Ln(5)
}

func (s *ReportService) renderSummarySection(pdf *gofpdf.Fpdf, summary *models.RunResultSummary) {
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.CellFormat(0, 8, "Results Summary", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(0, 0, 0)

	summaryFields := []struct {
		label string
		value string
	}{
		{"Total Pods Spawned", fmt.Sprintf("%d", summary.TotalPodsSpawned)},
		{"Successful Attacks", fmt.Sprintf("%d", summary.SuccessfulAttacks)},
		{"Blocked Attacks", fmt.Sprintf("%d", summary.BlockedAttacks)},
		{"Detection Rate", fmt.Sprintf("%.1f%%", summary.DetectionRate)},
		{"Overall Status", summary.OverallStatus},
	}

	for _, f := range summaryFields {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(55, 6, f.label, "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 6, f.value, "", 1, "L", false, 0, "")
	}

	// Findings
	if len(summary.Findings) > 0 {
		pdf.Ln(3)
		pdf.SetFont("Arial", "B", 11)
		pdf.SetTextColor(26, 54, 93)
		pdf.CellFormat(0, 6, "Key Findings", "", 1, "L", false, 0, "")

		pdf.SetFont("Arial", "", 10)
		pdf.SetTextColor(0, 0, 0)
		for _, finding := range summary.Findings {
			pdf.SetTextColor(220, 53, 69)
			pdf.CellFormat(5, 6, fmt.Sprintf("[%s]", finding.Severity), "", 0, "L", false, 0, "")
			pdf.SetTextColor(0, 0, 0)
			pdf.MultiCell(0, 5, finding.Description, "", "L", false)
			if finding.Recommendation != "" {
				pdf.SetFont("Arial", "I", 9)
				pdf.SetTextColor(100, 100, 100)
				pdf.MultiCell(0, 4, fmt.Sprintf("  Recommendation: %s", finding.Recommendation), "", "L", false)
				pdf.SetFont("Arial", "", 10)
				pdf.SetTextColor(0, 0, 0)
			}
		}
	}

	pdf.Ln(5)
}

func (s *ReportService) renderFooter(pdf *gofpdf.Fpdf) {
	pdf.SetY(-30)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(150, 150, 150)
	pdf.CellFormat(0, 5, "Generated by Chaos-Sec - Chaos Engineering for Kubernetes", "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("Report generated on %s", time.Now().Format("2006-01-02 15:04:05")), "", 1, "C", false, 0, "")
}

// GenerateJSONReport generates a JSON report for an experiment.
func (s *ReportService) GenerateJSONReport(ctx context.Context, experimentID uuid.UUID) (*ReportData, error) {
	return s.FetchReportData(ctx, experimentID)
}
