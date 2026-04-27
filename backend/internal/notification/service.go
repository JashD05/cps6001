package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/chaos-sec/backend/internal/models"
	"go.uber.org/zap"
)

// Service handles sending notifications through multiple channels (Email, Slack, Webhooks).
type Service struct {
	logger       *zap.Logger
	config       *Config
	smtpAuth     smtp.Auth
	emailEnabled bool
	slackEnabled bool
	webhookURL   string
	client       *http.Client
	templateDir  string
	channelMu    sync.RWMutex
}

// Config holds notification service configuration.
type Config struct {
	// Email settings
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from"`
	SMTPFromName string `json:"smtp_from_name"`

	// Slack settings
	SlackWebhookURL string `json:"slack_webhook_url"`
	SlackChannel    string `json:"slack_channel"`
	SlackUsername   string `json:"slack_username"`

	// Generic webhook
	WebhookURL string `json:"webhook_url"`

	// General settings
	Enabled    bool `json:"enabled"`
	AsyncSend  bool `json:"async_send"`
	RetryCount int  `json:"retry_count"`
	TimeoutSec int  `json:"timeout_sec"`
}

// NotificationEvent represents an event that triggers a notification.
type NotificationEvent struct {
	Type      string                   `json:"type"` // experiment_completed, experiment_failed, experiment_started, etc.
	Title     string                   `json:"title"`
	Message   string                   `json:"message"`
	RunID     string                   `json:"run_id,omitempty"`
	ExpID     string                   `json:"experiment_id,omitempty"`
	Status    string                   `json:"status,omitempty"`
	Summary   *models.RunResultSummary `json:"summary,omitempty"`
	Errors    []string                 `json:"errors,omitempty"`
	Timestamp time.Time                `json:"timestamp"`
	Metadata  map[string]interface{}   `json:"metadata,omitempty"`
}

// NotificationResult holds the result of sending a notification.
type NotificationResult struct {
	Channel   string    `json:"channel"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
	RetryUsed bool      `json:"retry_used"`
}

// NewService creates a new notification service with the given configuration.
func NewService(cfg *Config, logger *zap.Logger) *Service {
	s := &Service{
		logger:      logger,
		config:      cfg,
		client:      &http.Client{Timeout: 30 * time.Second},
		templateDir: "./templates",
	}

	if cfg != nil {
		if cfg.SMTPHost != "" && cfg.SMTPUsername != "" {
			s.smtpAuth = smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
			s.emailEnabled = true
		}
		s.slackEnabled = cfg.SlackWebhookURL != ""
		s.webhookURL = cfg.WebhookURL
	}

	if s.logger == nil {
		s.logger = zap.NewNop()
	}

	return s
}

// SendNotification sends a notification through all configured channels.
func (s *Service) SendNotification(ctx context.Context, event NotificationEvent) []NotificationResult {
	results := make([]NotificationResult, 0)

	if s.config != nil && !s.config.Enabled {
		s.logger.Debug("notifications disabled, skipping")
		return results
	}

	event.Timestamp = time.Now()

	if s.config != nil && s.config.AsyncSend {
		go s.sendAsync(event)
		return results
	}

	s.channelMu.RLock()
	defer s.channelMu.RUnlock()

	if s.emailEnabled {
		result := s.sendEmail(ctx, event)
		results = append(results, result)
	}

	if s.slackEnabled {
		result := s.sendSlack(ctx, event)
		results = append(results, result)
	}

	if s.webhookURL != "" {
		result := s.sendWebhook(ctx, event)
		results = append(results, result)
	}

	return results
}

// sendAsync sends notifications asynchronously.
func (s *Service) sendAsync(event NotificationEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	s.SendNotification(ctx, event)
}

// sendEmail sends an email notification.
func (s *Service) sendEmail(ctx context.Context, event NotificationEvent) NotificationResult {
	if !s.emailEnabled {
		return NotificationResult{Channel: "email", Success: false, Error: "email not enabled"}
	}

	subject := s.buildEmailSubject(event)
	body := s.buildEmailBody(event)

	from := s.config.SMTPFrom
	if s.config.SMTPFromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.SMTPFromName, s.config.SMTPFrom)
	}

	msg := s.buildEmailMessage(from, subject, body)

	var lastErr error
	retryCount := 3
	if s.config != nil && s.config.RetryCount > 0 {
		retryCount = s.config.RetryCount
	}

	for attempt := 0; attempt < retryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second) // exponential backoff
		}

		err := s.sendEmailWithRetry(ctx, msg)
		if err == nil {
			s.logger.Info("email notification sent successfully",
				zap.String("subject", subject),
				zap.String("run_id", event.RunID),
			)
			return NotificationResult{
				Channel:   "email",
				Success:   true,
				Timestamp: time.Now(),
			}
		}
		lastErr = err
		s.logger.Warn("email send attempt failed",
			zap.Int("attempt", attempt+1),
			zap.Error(err),
		)
	}

	s.logger.Error("failed to send email notification after all retries",
		zap.String("subject", subject),
		zap.Error(lastErr),
	)

	return NotificationResult{
		Channel:   "email",
		Success:   false,
		Timestamp: time.Now(),
		Error:     lastErr.Error(),
		RetryUsed: retryCount > 1,
	}
}

func (s *Service) sendEmailWithRetry(ctx context.Context, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)

	var dialErr error
	var client *smtp.Client
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		client, dialErr = smtp.Dial(addr)
		if dialErr == nil {
			break
		}
	}

	if dialErr != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", dialErr)
	}
	defer client.Close()

	if err := client.StartTLS(nil); err != nil {
		// Try without TLS for development
		s.logger.Debug("TLS handshake failed, continuing without TLS", zap.Error(err))
	}

	if err := client.Auth(s.smtpAuth); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}

	if err := client.Mail(s.config.SMTPUsername); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Get recipients from the message
	lines := strings.Split(string(msg), "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "To:") {
			addrs := strings.TrimPrefix(line, "To:")
			for _, addr := range strings.Split(addrs, ",") {
				addr = strings.TrimSpace(addr)
				if addr != "" {
					if err := client.Rcpt(addr); err != nil {
						return fmt.Errorf("failed to set recipient %s: %w", addr, err)
					}
				}
			}
			break
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open data writer: %w", err)
	}

	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to write message body: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	if err := client.Quit(); err != nil {
		s.logger.Debug("QUIT failed (may be already closed)", zap.Error(err))
	}

	return nil
}

func (s *Service) buildEmailMessage(from, subject, body string) []byte {
	var buf bytes.Buffer

	// Set From
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))

	// Set To (use a placeholder - actual recipients should be configured)
	toList := "admin@example.com"
	buf.WriteString(fmt.Sprintf("To: %s\r\n", toList))

	// Set Subject
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))

	// Set Headers
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString("\r\n")

	// Add HTML body
	buf.WriteString(body)

	return buf.Bytes()
}

func (s *Service) buildEmailSubject(event NotificationEvent) string {
	switch event.Type {
	case "experiment_completed":
		return fmt.Sprintf("✅ Experiment Completed Successfully - %s", event.Title)
	case "experiment_failed":
		return fmt.Sprintf("❌ Experiment Failed - %s", event.Title)
	case "experiment_started":
		return fmt.Sprintf("🚀 Experiment Started - %s", event.Title)
	case "experiment_cancelled":
		return fmt.Sprintf("⚠️ Experiment Cancelled - %s", event.Title)
	case "siem_alert_missed":
		return fmt.Sprintf("🚨 SIEM Alert Missed - Security Gap Detected")
	default:
		return fmt.Sprintf("Chaos-Sec Notification - %s", event.Title)
	}
}

func (s *Service) buildEmailBody(event NotificationEvent) string {
	var buf bytes.Buffer

	buf.WriteString("<!DOCTYPE html><html><head><style>")
	buf.WriteString("body{font-family:Arial,sans-serif;margin:0;padding:20px;background:#f5f5f5;}")
	buf.WriteString(".container{max-width:600px;margin:0 auto;background:#fff;border-radius:8px;")
	buf.WriteString("box-shadow:0 2px 10px rgba(0,0,0,0.1);overflow:hidden;}")
	buf.WriteString(".header{padding:20px 30px;background:#1a365d;color:#fff;}")
	buf.WriteString(".header h1{margin:0;font-size:20px;}")
	buf.WriteString(".content{padding:30px;}")
	buf.WriteString(".status{padding:10px 15px;border-radius:4px;margin:10px 0;}")
	buf.WriteString(".status.success{background:#d4edda;color:#155724;}")
	buf.WriteString(".status.error{background:#f8d7da;color:#721c24;}")
	buf.WriteString(".status.info{background:#d1ecf1;color:#0c5460;}")
	buf.WriteString(".details{margin:20px 0;}")
	buf.WriteString(".details table{width:100%;border-collapse:collapse;}")
	buf.WriteString(".details td{padding:8px 0;border-bottom:1px solid #eee;}")
	buf.WriteString(".details td:first-child{font-weight:bold;width:40%;}")
	buf.WriteString(".footer{padding:15px 30px;background:#f8f9fa;text-align:center;")
	buf.WriteString("color:#6c757d;font-size:12px;}")
	buf.WriteString("</style></head><body>")

	buf.WriteString("<div class=\"container\">")
	buf.WriteString("<div class=\"header\"><h1>🔬 Chaos-Sec Notification</h1></div>")
	buf.WriteString("<div class=\"content\">")

	// Status badge
	statusClass := "info"
	if event.Type == "experiment_completed" {
		statusClass = "success"
	} else if event.Type == "experiment_failed" || event.Type == "siem_alert_missed" {
		statusClass = "error"
	}
	buf.WriteString(fmt.Sprintf("<div class=\"status %s\">%s</div>", statusClass, event.Title))

	// Message
	if event.Message != "" {
		buf.WriteString(fmt.Sprintf("<p>%s</p>", event.Message))
	}

	// Details table
	buf.WriteString("<div class=\"details\"><table>")

	if event.RunID != "" {
		buf.WriteString(fmt.Sprintf("<tr><td>Run ID</td><td>%s</td></tr>", event.RunID))
	}
	if event.ExpID != "" {
		buf.WriteString(fmt.Sprintf("<tr><td>Experiment ID</td><td>%s</td></tr>", event.ExpID))
	}
	if event.Status != "" {
		buf.WriteString(fmt.Sprintf("<tr><td>Status</td><td>%s</td></tr>", event.Status))
	}

	buf.WriteString(fmt.Sprintf("<tr><td>Time</td><td>%s</td></tr>", event.Timestamp.Format("2006-01-02 15:04:05")))

	buf.WriteString("</table></div>")

	// Summary section
	if event.Summary != nil {
		buf.WriteString("<h3>Results Summary</h3>")
		buf.WriteString("<table>")
		buf.WriteString(fmt.Sprintf("<tr><td>Total Pods</td><td>%d</td></tr>", event.Summary.TotalPodsSpawned))
		buf.WriteString(fmt.Sprintf("<tr><td>Successful Attacks</td><td>%d</td></tr>", event.Summary.SuccessfulAttacks))
		buf.WriteString(fmt.Sprintf("<tr><td>Blocked Attacks</td><td>%d</td></tr>", event.Summary.BlockedAttacks))
		buf.WriteString(fmt.Sprintf("<tr><td>Detection Rate</td><td>%.1f%%</td></tr>", event.Summary.DetectionRate))
		buf.WriteString(fmt.Sprintf("<tr><td>Overall Status</td><td>%s</td></tr>", event.Summary.OverallStatus))
		buf.WriteString("</table>")
	}

	// Errors
	if len(event.Errors) > 0 {
		buf.WriteString("<h3>Errors</h3><ul>")
		for _, err := range event.Errors {
			buf.WriteString(fmt.Sprintf("<li>%s</li>", err))
		}
		buf.WriteString("</ul>")
	}

	buf.WriteString("</div>")
	buf.WriteString("<div class=\"footer\">Generated by Chaos-Sec | Chaos Engineering Platform</div>")
	buf.WriteString("</div></body></html>")

	return buf.String()
}

// sendSlack sends a Slack notification.
func (s *Service) sendSlack(ctx context.Context, event NotificationEvent) NotificationResult {
	if !s.slackEnabled {
		return NotificationResult{Channel: "slack", Success: false, Error: "slack not enabled"}
	}

	payload := s.buildSlackPayload(event)
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return NotificationResult{Channel: "slack", Success: false, Error: fmt.Sprintf("failed to marshal payload: %v", err)}
	}

	var lastErr error
	retryCount := 3
	if s.config != nil && s.config.RetryCount > 0 {
		retryCount = s.config.RetryCount
	}

	for attempt := 0; attempt < retryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", s.config.SlackWebhookURL, bytes.NewBuffer(jsonPayload))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			s.logger.Info("slack notification sent successfully",
				zap.String("run_id", event.RunID),
			)
			return NotificationResult{
				Channel:   "slack",
				Success:   true,
				Timestamp: time.Now(),
			}
		}

		lastErr = fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	s.logger.Error("failed to send slack notification after all retries",
		zap.String("run_id", event.RunID),
		zap.Error(lastErr),
	)

	return NotificationResult{
		Channel:   "slack",
		Success:   false,
		Timestamp: time.Now(),
		Error:     lastErr.Error(),
		RetryUsed: retryCount > 1,
	}
}

func (s *Service) buildSlackPayload(event NotificationEvent) map[string]interface{} {
	color := "#36a64f" // green - success
	emoji := "✅"

	switch event.Type {
	case "experiment_failed":
		color = "#ff0000"
		emoji = "❌"
	case "experiment_started":
		color = "#439fd3"
		emoji = "🚀"
	case "experiment_cancelled":
		color = "#ff9800"
		emoji = "⚠️"
	case "siem_alert_missed":
		color = "#ff0000"
		emoji = "🚨"
	}

	fields := make([]map[string]interface{}, 0)
	if event.RunID != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Run ID",
			"value": event.RunID,
			"short": true,
		})
	}
	if event.Status != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Status",
			"value": event.Status,
			"short": true,
		})
	}
	fields = append(fields, map[string]interface{}{
		"title": "Time",
		"value": event.Timestamp.Format("2006-01-02 15:04:05"),
		"short": true,
	})

	// Add summary if available
	if event.Summary != nil {
		fields = append(fields,
			map[string]interface{}{"title": "Total Pods", "value": event.Summary.TotalPodsSpawned, "short": true},
			map[string]interface{}{"title": "Successful", "value": event.Summary.SuccessfulAttacks, "short": true},
			map[string]interface{}{"title": "Blocked", "value": event.Summary.BlockedAttacks, "short": true},
			map[string]interface{}{"title": "Detection Rate", "value": fmt.Sprintf("%.1f%%", event.Summary.DetectionRate), "short": true},
		)
	}

	payload := map[string]interface{}{
		"username": s.config.SlackUsername,
		"channel":  s.config.SlackChannel,
		"attachments": []map[string]interface{}{
			{
				"color":       color,
				"fallback":    fmt.Sprintf("%s %s", emoji, event.Title),
				"author_name": "Chaos-Sec",
				"title":       fmt.Sprintf("%s %s", emoji, event.Title),
				"text":        event.Message,
				"fields":      fields,
				"footer":      "Chaos-Sec Notification System",
				"ts":          event.Timestamp.Unix(),
			},
		},
	}

	return payload
}

// sendWebhook sends a generic webhook notification.
func (s *Service) sendWebhook(ctx context.Context, event NotificationEvent) NotificationResult {
	if s.webhookURL == "" {
		return NotificationResult{Channel: "webhook", Success: false, Error: "webhook URL not configured"}
	}

	payload := map[string]interface{}{
		"event_type": event.Type,
		"title":      event.Title,
		"message":    event.Message,
		"run_id":     event.RunID,
		"exp_id":     event.ExpID,
		"status":     event.Status,
		"timestamp":  event.Timestamp.Format(time.RFC3339),
		"summary":    event.Summary,
		"errors":     event.Errors,
		"metadata":   event.Metadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return NotificationResult{Channel: "webhook", Success: false, Error: fmt.Sprintf("failed to marshal payload: %v", err)}
	}

	var lastErr error
	retryCount := 3
	if s.config != nil && s.config.RetryCount > 0 {
		retryCount = s.config.RetryCount
	}

	for attempt := 0; attempt < retryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewBuffer(jsonPayload))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		defer resp.Body.Close()

		// Read response body to prevent connection leak
		io.Copy(io.Discard, resp.Body)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			s.logger.Info("webhook notification sent successfully",
				zap.String("run_id", event.RunID),
				zap.String("url", s.webhookURL),
			)
			return NotificationResult{
				Channel:   "webhook",
				Success:   true,
				Timestamp: time.Now(),
			}
		}

		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	s.logger.Error("failed to send webhook notification after all retries",
		zap.String("run_id", event.RunID),
		zap.String("url", s.webhookURL),
		zap.Error(lastErr),
	)

	return NotificationResult{
		Channel:   "webhook",
		Success:   false,
		Timestamp: time.Now(),
		Error:     lastErr.Error(),
		RetryUsed: retryCount > 1,
	}
}

// IsEnabled returns whether any notification channel is enabled.
func (s *Service) IsEnabled() bool {
	s.channelMu.RLock()
	defer s.channelMu.RUnlock()
	return s.emailEnabled || s.slackEnabled || s.webhookURL != ""
}

// GetChannels returns a list of enabled notification channels.
func (s *Service) GetChannels() []string {
	s.channelMu.RLock()
	defer s.channelMu.RUnlock()

	channels := make([]string, 0)
	if s.emailEnabled {
		channels = append(channels, "email")
	}
	if s.slackEnabled {
		channels = append(channels, "slack")
	}
	if s.webhookURL != "" {
		channels = append(channels, "webhook")
	}
	return channels
}
