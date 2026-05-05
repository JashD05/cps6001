package experiment

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// GenerateCSVReport generates a CSV report for an experiment.
func (s *ReportService) GenerateCSVReport(ctx context.Context, experimentID uuid.UUID) ([]byte, error) {
	data, err := s.FetchReportData(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch report data: %w", err)
	}

	var buf bytes.Buffer

	// Header row
	buf.WriteString("Field,Value\n")
	buf.WriteString(fmt.Sprintf("Experiment Name,%s\n", escapeCSV(data.Experiment.Name)))
	buf.WriteString(fmt.Sprintf("Experiment ID,%s\n", data.Experiment.ID.String()))
	buf.WriteString(fmt.Sprintf("Status,%s\n", data.Experiment.Status))
	buf.WriteString(fmt.Sprintf("Description,%s\n", escapeCSV(data.Experiment.Description)))
	buf.WriteString(fmt.Sprintf("Created At,%s\n", data.Experiment.CreatedAt.Format(time.RFC3339)))
	buf.WriteString(fmt.Sprintf("Generated At,%s\n", data.GeneratedAt.Format(time.RFC3339)))
	buf.WriteString("\n")

	// Runs section
	if len(data.Runs) > 0 {
		buf.WriteString("Run Number,Status,Triggered By,Trigger Type,Started At,Completed At,Duration (ms),Error Message\n")
		for _, run := range data.Runs {
			startedAt := ""
			if run.StartedAt != nil && !run.StartedAt.IsZero() {
				startedAt = run.StartedAt.Format(time.RFC3339)
			}
			completedAt := ""
			if run.CompletedAt != nil && !run.CompletedAt.IsZero() {
				completedAt = run.CompletedAt.Format(time.RFC3339)
			}
			durationMs := ""
			if run.DurationMs != nil {
				durationMs = fmt.Sprintf("%d", *run.DurationMs)
			}
			triggeredBy := ""
			if run.TriggeredBy != nil {
				triggeredBy = run.TriggeredBy.String()
			}
			errorMsg := ""
			if run.ErrorMessage != nil {
				errorMsg = escapeCSV(*run.ErrorMessage)
			}
			buf.WriteString(fmt.Sprintf("%d,%s,%s,%s,%s,%s,%s,%s\n",
				run.RunNumber, run.Status, triggeredBy, run.TriggerType,
				startedAt, completedAt, durationMs, errorMsg))
		}
		buf.WriteString("\n")
	}

	// Summary section
	if data.Summary != nil {
		buf.WriteString("Summary Field,Value\n")
		buf.WriteString(fmt.Sprintf("Total Pods Spawned,%d\n", data.Summary.TotalPodsSpawned))
		buf.WriteString(fmt.Sprintf("Successful Attacks,%d\n", data.Summary.SuccessfulAttacks))
		buf.WriteString(fmt.Sprintf("Blocked Attacks,%d\n", data.Summary.BlockedAttacks))
		buf.WriteString(fmt.Sprintf("Detection Rate,%.1f%%\n", data.Summary.DetectionRate))
		buf.WriteString(fmt.Sprintf("Overall Status,%s\n", data.Summary.OverallStatus))

		if len(data.Summary.Findings) > 0 {
			buf.WriteString("\nSeverity,Description,Recommendation\n")
			for _, f := range data.Summary.Findings {
				buf.WriteString(fmt.Sprintf("%s,%s,%s\n",
					f.Severity, escapeCSV(f.Description), escapeCSV(f.Recommendation)))
			}
		}
	}

	return buf.Bytes(), nil
}

// GenerateHTMLReport generates an HTML report for an experiment.
func (s *ReportService) GenerateHTMLReport(ctx context.Context, experimentID uuid.UUID) ([]byte, error) {
	data, err := s.FetchReportData(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch report data: %w", err)
	}

	var buf bytes.Buffer

	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Chaos-Sec Experiment Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; color: #1a365d; margin: 0; padding: 40px; background: #f7fafc; }
  .container { max-width: 900px; margin: 0 auto; background: #fff; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.12); padding: 40px; }
  h1 { color: #1a365d; border-bottom: 2px solid #1a365d; padding-bottom: 10px; }
  h2 { color: #1a365d; margin-top: 30px; }
  .meta { color: #718096; font-size: 0.9em; margin-bottom: 20px; }
  .field { margin: 4px 0; }
  .field-label { font-weight: 600; display: inline-block; width: 180px; }
  table { width: 100%; border-collapse: collapse; margin-top: 10px; }
  th { background: #1a365d; color: #fff; padding: 10px 12px; text-align: left; font-size: 0.85em; }
  td { padding: 8px 12px; border-bottom: 1px solid #e2e8f0; font-size: 0.85em; }
  tr:nth-child(even) { background: #f7fafc; }
  .status-completed { color: #38a169; font-weight: 600; }
  .status-error, .status-failed { color: #e53e3e; font-weight: 600; }
  .severity { padding: 2px 6px; border-radius: 3px; font-size: 0.8em; font-weight: 600; }
  .severity-high { background: #fed7d7; color: #c53030; }
  .severity-medium { background: #fefcbf; color: #975a16; }
  .severity-low { background: #c6f6d5; color: #276749; }
  .footer { margin-top: 40px; padding-top: 15px; border-top: 1px solid #e2e8f0; color: #a0aec0; font-size: 0.8em; text-align: center; }
</style>
</head>
<body>
<div class="container">
`)

	// Header
	buf.WriteString(fmt.Sprintf("<h1>Chaos-Sec Experiment Report</h1>\n"))
	buf.WriteString(fmt.Sprintf("<p class=\"meta\">Generated: %s</p>\n", data.GeneratedAt.Format(time.RFC1123)))

	// Experiment details
	buf.WriteString("<h2>Experiment Details</h2>\n")
	buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Name:</span> %s</div>\n", htmlEsc(data.Experiment.Name)))
	buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">ID:</span> %s</div>\n", data.Experiment.ID.String()))
	buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Status:</span> %s</div>\n", htmlEsc(data.Experiment.Status)))
	if data.Experiment.Description != "" {
		buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Description:</span> %s</div>\n", htmlEsc(data.Experiment.Description)))
	}
	buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Created:</span> %s</div>\n", data.Experiment.CreatedAt.Format("2006-01-02 15:04:05")))

	// Runs table
	if len(data.Runs) > 0 {
		buf.WriteString("<h2>Experiment Runs</h2>\n")
		buf.WriteString("<table><thead><tr><th>Run #</th><th>Status</th><th>Triggered By</th><th>Started</th><th>Duration</th><th>Result</th></tr></thead><tbody>\n")
		for _, run := range data.Runs {
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
			result := "N/A"
			if run.ErrorMessage != nil && *run.ErrorMessage != "" {
				result = "Error"
			} else if run.Status == "completed" {
				result = "Success"
			}
			statusClass := ""
			switch run.Status {
			case "completed":
				statusClass = "status-completed"
			case "failed", "error":
				statusClass = "status-error"
			}
			buf.WriteString(fmt.Sprintf("<tr><td>Run %d</td><td class=\"%s\">%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				run.RunNumber, statusClass, htmlEsc(run.Status), htmlEsc(triggeredBy), started, duration, result))
		}
		buf.WriteString("</tbody></table>\n")
	}

	// Summary section
	if data.Summary != nil {
		buf.WriteString("<h2>Results Summary</h2>\n")
		buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Total Pods Spawned:</span> %d</div>\n", data.Summary.TotalPodsSpawned))
		buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Successful Attacks:</span> %d</div>\n", data.Summary.SuccessfulAttacks))
		buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Blocked Attacks:</span> %d</div>\n", data.Summary.BlockedAttacks))
		buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Detection Rate:</span> %.1f%%</div>\n", data.Summary.DetectionRate))
		buf.WriteString(fmt.Sprintf("<div class=\"field\"><span class=\"field-label\">Overall Status:</span> %s</div>\n", htmlEsc(data.Summary.OverallStatus)))

		if len(data.Summary.Findings) > 0 {
			buf.WriteString("<h3>Key Findings</h3>\n")
			for _, f := range data.Summary.Findings {
				severityClass := "severity-low"
				switch f.Severity {
				case "high", "critical":
					severityClass = "severity-high"
				case "medium":
					severityClass = "severity-medium"
				}
				buf.WriteString(fmt.Sprintf("<p><span class=\"severity %s\">%s</span> %s</p>\n", severityClass, htmlEsc(f.Severity), htmlEsc(f.Description)))
				if f.Recommendation != "" {
					buf.WriteString(fmt.Sprintf("<p style=\"color:#718096;font-size:0.9em;margin-left:20px;\"><em>Recommendation: %s</em></p>\n", htmlEsc(f.Recommendation)))
				}
			}
		}
	}

	// Footer
	buf.WriteString(`<div class="footer">Generated by Chaos-Sec — Chaos Engineering for Kubernetes</div>
</div>
</body>
</html>`)

	return buf.Bytes(), nil
}

// escapeCSV escapes a string for safe inclusion in a CSV field.
func escapeCSV(s string) string {
	if strings.Contains(s, ",") || strings.Contains(s, "\"") || strings.Contains(s, "\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// htmlEsc escapes a string for safe inclusion in HTML.
func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
