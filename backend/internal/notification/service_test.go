package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chaos-sec/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewService(t *testing.T) {
	cfg := &Config{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUsername: "user@example.com",
		SMTPPassword: "password",
		SMTPFrom:     "chaos-sec@example.com",
		Enabled:      true,
	}

	logger, _ := zap.NewDevelopment()
	svc := NewService(cfg, logger)

	assert.NotNil(t, svc)
	assert.True(t, svc.emailEnabled)
	assert.False(t, svc.slackEnabled)
	assert.Empty(t, svc.webhookURL)
}

func TestNewService_NoEmailConfig(t *testing.T) {
	cfg := &Config{
		SlackWebhookURL: "https://hooks.slack.com/services/xxx",
		SlackUsername:   "chaos-sec-bot",
		Enabled:         true,
	}

	logger, _ := zap.NewDevelopment()
	svc := NewService(cfg, logger)

	assert.NotNil(t, svc)
	assert.False(t, svc.emailEnabled)
	assert.True(t, svc.slackEnabled)
	assert.Empty(t, svc.webhookURL)
}

func TestNewService_NilConfig(t *testing.T) {
	svc := NewService(nil, nil)
	assert.NotNil(t, svc)
	assert.False(t, svc.emailEnabled)
	assert.False(t, svc.slackEnabled)
}

func TestNewService_NoChannels(t *testing.T) {
	cfg := &Config{Enabled: true}
	svc := NewService(cfg, nil)
	assert.NotNil(t, svc)
	assert.False(t, svc.emailEnabled)
	assert.False(t, svc.slackEnabled)
	assert.Empty(t, svc.webhookURL)
}

func TestService_IsEnabled(t *testing.T) {
	t.Run("email enabled", func(t *testing.T) {
		cfg := &Config{
			SMTPHost:     "smtp.example.com",
			SMTPUsername: "user",
			SMTPPassword: "pass",
			Enabled:      true,
		}
		svc := NewService(cfg, nil)
		assert.True(t, svc.IsEnabled())
	})

	t.Run("slack enabled", func(t *testing.T) {
		cfg := &Config{
			SlackWebhookURL: "https://hooks.slack.com/xxx",
			Enabled:         true,
		}
		svc := NewService(cfg, nil)
		assert.True(t, svc.IsEnabled())
	})

	t.Run("webhook enabled", func(t *testing.T) {
		cfg := &Config{
			WebhookURL: "https://example.com/webhook",
			Enabled:    true,
		}
		svc := NewService(cfg, nil)
		assert.True(t, svc.IsEnabled())
	})

	t.Run("no channels", func(t *testing.T) {
		svc := NewService(&Config{Enabled: true}, nil)
		assert.False(t, svc.IsEnabled())
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := &Config{
			Enabled: false,
		}
		svc := NewService(cfg, nil)
		assert.False(t, svc.IsEnabled())
	})
}

func TestService_GetChannels(t *testing.T) {
	t.Run("all channels", func(t *testing.T) {
		cfg := &Config{
			SMTPHost:        "smtp.example.com",
			SMTPUsername:    "user",
			SMTPPassword:    "pass",
			SlackWebhookURL: "https://hooks.slack.com/xxx",
			WebhookURL:      "https://example.com/webhook",
			Enabled:         true,
		}
		svc := NewService(cfg, nil)
		channels := svc.GetChannels()
		assert.Len(t, channels, 3)
		assert.Contains(t, channels, "email")
		assert.Contains(t, channels, "slack")
		assert.Contains(t, channels, "webhook")
	})

	t.Run("email only", func(t *testing.T) {
		cfg := &Config{
			SMTPHost:     "smtp.example.com",
			SMTPUsername: "user",
			SMTPPassword: "pass",
			Enabled:      true,
		}
		svc := NewService(cfg, nil)
		channels := svc.GetChannels()
		assert.Len(t, channels, 1)
		assert.Contains(t, channels, "email")
	})

	t.Run("none", func(t *testing.T) {
		svc := NewService(&Config{Enabled: true}, nil)
		channels := svc.GetChannels()
		assert.Empty(t, channels)
	})
}

func TestService_SendNotification_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:    "experiment_completed",
		Title:   "Test",
		Message: "Test message",
	}

	results := svc.SendNotification(context.Background(), event)
	assert.Empty(t, results)
}

func TestService_SendNotification_Async(t *testing.T) {
	// Create a test server to capture Slack webhook
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		SlackWebhookURL: server.URL,
		AsyncSend:       true,
		Enabled:         true,
	}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:    "experiment_completed",
		Title:   "Test Experiment",
		Message: "Completed successfully",
		RunID:   "run-123",
	}

	results := svc.SendNotification(context.Background(), event)
	assert.Empty(t, results) // async returns empty immediately
}

func TestBuildSlackPayload(t *testing.T) {
	cfg := &Config{
		SlackWebhookURL: "https://hooks.slack.com/xxx",
		SlackUsername:   "test-bot",
		SlackChannel:    "#test",
	}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_completed",
		Title:     "Test Completed",
		Message:   "Experiment finished successfully",
		RunID:     "run-456",
		Status:    "completed",
		Timestamp: time.Now(),
		Summary: &models.RunResultSummary{
			TotalPodsSpawned:  5,
			SuccessfulAttacks: 3,
			BlockedAttacks:    2,
			DetectionRate:     80.0,
			OverallStatus:     "passed",
		},
	}

	payload := svc.buildSlackPayload(event)

	assert.Equal(t, "test-bot", payload["username"])
	assert.Equal(t, "#test", payload["channel"])

	attachments, ok := payload["attachments"].([]map[string]interface{})
	assert.True(t, ok)
	assert.Len(t, attachments, 1)

	attachment := attachments[0]
	assert.Equal(t, "#36a64f", attachment["color"])
	assert.Equal(t, "✅ Test Completed", attachment["title"])
}

func TestBuildSlackPayload_Failed(t *testing.T) {
	cfg := &Config{SlackWebhookURL: "https://hooks.slack.com/xxx"}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_failed",
		Title:     "Test Failed",
		Message:   "Experiment failed",
		Timestamp: time.Now(),
	}

	payload := svc.buildSlackPayload(event)
	attachments := payload["attachments"].([]map[string]interface{})
	assert.Equal(t, "#ff0000", attachments[0]["color"])
	assert.Contains(t, attachments[0]["title"], "❌")
}

func TestBuildSlackPayload_Started(t *testing.T) {
	cfg := &Config{SlackWebhookURL: "https://hooks.slack.com/xxx"}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_started",
		Title:     "Test Started",
		Timestamp: time.Now(),
	}

	payload := svc.buildSlackPayload(event)
	attachments := payload["attachments"].([]map[string]interface{})
	assert.Equal(t, "#439fd3", attachments[0]["color"])
}

func TestBuildSlackPayload_SIEMMissed(t *testing.T) {
	cfg := &Config{SlackWebhookURL: "https://hooks.slack.com/xxx"}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "siem_alert_missed",
		Title:     "Security Alert Missed",
		Timestamp: time.Now(),
	}

	payload := svc.buildSlackPayload(event)
	attachments := payload["attachments"].([]map[string]interface{})
	assert.Equal(t, "#ff0000", attachments[0]["color"])
	assert.Contains(t, attachments[0]["title"], "🚨")
}

func TestService_SendSlack_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		SlackWebhookURL: server.URL,
		Enabled:         true,
	}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_completed",
		Title:     "Test",
		Timestamp: time.Now(),
	}

	result := svc.sendSlack(context.Background(), event)
	assert.True(t, result.Success)
	assert.Equal(t, "slack", result.Channel)
	assert.Empty(t, result.Error)
}

func TestService_SendSlack_NotEnabled(t *testing.T) {
	cfg := &Config{Enabled: true}
	svc := NewService(cfg, nil)

	event := NotificationEvent{Type: "test", Timestamp: time.Now()}
	result := svc.sendSlack(context.Background(), event)

	assert.False(t, result.Success)
	assert.Equal(t, "slack", result.Channel)
	assert.NotEmpty(t, result.Error)
}

func TestService_SendSlack_RetryAndFail(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := &Config{
		SlackWebhookURL: server.URL,
		RetryCount:      3,
		Enabled:         true,
	}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_completed",
		Title:     "Test",
		Timestamp: time.Now(),
	}

	result := svc.sendSlack(context.Background(), event)
	assert.False(t, result.Success)
	assert.Equal(t, 3, requestCount)
	assert.True(t, result.RetryUsed)
}

func TestService_SendWebhook_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		assert.Equal(t, "experiment_completed", payload["event_type"])
		assert.Equal(t, "Test Title", payload["title"])

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		WebhookURL: server.URL,
		Enabled:    true,
	}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:    "experiment_completed",
		Title:   "Test Title",
		Message: "Test message",
		RunID:   "run-789",
		ExpID:   "exp-123",
		Status:  "completed",
	}

	result := svc.sendWebhook(context.Background(), event)
	assert.True(t, result.Success)
	assert.Equal(t, "webhook", result.Channel)
}

func TestService_SendWebhook_NotConfigured(t *testing.T) {
	cfg := &Config{Enabled: true}
	svc := NewService(cfg, nil)

	event := NotificationEvent{Type: "test", Timestamp: time.Now()}
	result := svc.sendWebhook(context.Background(), event)

	assert.False(t, result.Success)
	assert.Equal(t, "webhook", result.Channel)
	assert.Contains(t, result.Error, "not configured")
}

func TestService_SendWebhook_RetryAndFail(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &Config{
		WebhookURL: server.URL,
		RetryCount: 3,
		Enabled:    true,
	}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_failed",
		Title:     "Failed",
		Timestamp: time.Now(),
	}

	result := svc.sendWebhook(context.Background(), event)
	assert.False(t, result.Success)
	assert.Equal(t, 3, requestCount)
	assert.True(t, result.RetryUsed)
}

func TestBuildEmailSubject(t *testing.T) {
	cfg := &Config{SMTPHost: "smtp.example.com", SMTPUsername: "u", SMTPPassword: "p"}
	svc := NewService(cfg, nil)

	tests := []struct {
		eventType string
		title     string
		expected  string
	}{
		{"experiment_completed", "Test", "✅ Experiment Completed Successfully - Test"},
		{"experiment_failed", "Test", "❌ Experiment Failed - Test"},
		{"experiment_started", "Test", "🚀 Experiment Started - Test"},
		{"experiment_cancelled", "Test", "⚠️ Experiment Cancelled - Test"},
		{"siem_alert_missed", "", "🚨 SIEM Alert Missed - Security Gap Detected"},
		{"unknown_type", "Test", "Chaos-Sec Notification - Test"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			event := NotificationEvent{Type: tt.eventType, Title: tt.title}
			subject := svc.buildEmailSubject(event)
			assert.Equal(t, tt.expected, subject)
		})
	}
}

func TestBuildEmailMessage(t *testing.T) {
	cfg := &Config{SMTPHost: "smtp.example.com", SMTPUsername: "u", SMTPPassword: "p"}
	svc := NewService(cfg, nil)

	subject := "Test Subject"
	body := "<html><body><p>Test body</p></body></html>"
	from := "Chaos Sec <no-reply@chaos-sec.com>"

	msg := svc.buildEmailMessage(from, subject, body)
	msgStr := string(msg)

	assert.Contains(t, msgStr, "From: "+from)
	assert.Contains(t, msgStr, "Subject: "+subject)
	assert.Contains(t, msgStr, "MIME-Version: 1.0")
	assert.Contains(t, msgStr, "Content-Type: text/html")
	assert.Contains(t, msgStr, body)
}

func TestBuildEmailBody(t *testing.T) {
	cfg := &Config{SMTPHost: "smtp.example.com", SMTPUsername: "u", SMTPPassword: "p"}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_completed",
		Title:     "Test Experiment Completed",
		Message:   "The experiment has completed successfully.",
		RunID:     "run-123",
		ExpID:     "exp-456",
		Status:    "completed",
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Summary: &models.RunResultSummary{
			TotalPodsSpawned:  10,
			SuccessfulAttacks: 8,
			BlockedAttacks:    2,
			DetectionRate:     85.0,
			OverallStatus:     "passed",
		},
	}

	body := svc.buildEmailBody(event)

	// Check HTML structure
	assert.Contains(t, body, "<!DOCTYPE html>")
	assert.Contains(t, body, "<div class=\"status success\">")
	assert.Contains(t, body, "Test Experiment Completed")
	assert.Contains(t, body, "The experiment has completed successfully.")
	assert.Contains(t, body, "run-123")
	assert.Contains(t, body, "exp-456")
	assert.Contains(t, body, "completed")
	assert.Contains(t, body, "2024-01-15 10:30:00")

	// Check summary
	assert.Contains(t, body, "10")    // Total Pods
	assert.Contains(t, body, "8")     // Successful Attacks
	assert.Contains(t, body, "2")     // Blocked Attacks
	assert.Contains(t, body, "85.0%") // Detection Rate
}

func TestBuildEmailBody_Failed(t *testing.T) {
	cfg := &Config{SMTPHost: "smtp.example.com", SMTPUsername: "u", SMTPPassword: "p"}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:    "experiment_failed",
		Title:   "Failed Experiment",
		Message: "Something went wrong",
		Errors:  []string{"Step 1 failed", "Step 2 timeout"},
	}

	body := svc.buildEmailBody(event)

	assert.Contains(t, body, "<div class=\"status error\">")
	assert.Contains(t, body, "Failed Experiment")
	assert.Contains(t, body, "Step 1 failed")
	assert.Contains(t, body, "Step 2 timeout")
}

func TestBuildEmailBody_NoSummary(t *testing.T) {
	cfg := &Config{SMTPHost: "smtp.example.com", SMTPUsername: "u", SMTPPassword: "p"}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_started",
		Title:     "Started",
		Timestamp: time.Now(),
	}

	body := svc.buildEmailBody(event)

	assert.Contains(t, body, "<div class=\"status info\">")
	assert.Contains(t, body, "Started")
	assert.NotContains(t, body, "Results Summary") // No summary section
}

func TestService_SendEmail_NotEnabled(t *testing.T) {
	cfg := &Config{Enabled: true}
	svc := NewService(cfg, nil)

	event := NotificationEvent{Type: "test", Title: "Test", Timestamp: time.Now()}
	result := svc.sendEmail(context.Background(), event)

	assert.False(t, result.Success)
	assert.Equal(t, "email", result.Channel)
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "not enabled")
}

func TestNotificationEvent_WithMetadata(t *testing.T) {
	event := NotificationEvent{
		Type:      "experiment_completed",
		Title:     "Test",
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"cluster_id":   "cluster-1",
			"region":       "us-east-1",
			"custom_field": "value",
		},
	}

	assert.Equal(t, "cluster-1", event.Metadata["cluster_id"])
	assert.Equal(t, "us-east-1", event.Metadata["region"])
}

func TestNotificationResult_Fields(t *testing.T) {
	now := time.Now()
	result := NotificationResult{
		Channel:   "email",
		Success:   true,
		Timestamp: now,
		Error:     "",
		RetryUsed: true,
	}

	assert.Equal(t, "email", result.Channel)
	assert.True(t, result.Success)
	assert.Equal(t, now, result.Timestamp)
	assert.Empty(t, result.Error)
	assert.True(t, result.RetryUsed)
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		SMTPHost:        "smtp.gmail.com",
		SMTPPort:        587,
		SMTPUsername:    "user@gmail.com",
		SMTPPassword:    "secret",
		SMTPFrom:        "alerts@chaos-sec.com",
		SMTPFromName:    "Chaos-Sec Alerts",
		SlackWebhookURL: "https://hooks.slack.com/xxx",
		SlackChannel:    "#alerts",
		SlackUsername:   "chaos-bot",
		WebhookURL:      "https://example.com/hook",
		Enabled:         true,
		AsyncSend:       true,
		RetryCount:      5,
		TimeoutSec:      30,
	}

	assert.Equal(t, "smtp.gmail.com", cfg.SMTPHost)
	assert.Equal(t, 587, cfg.SMTPPort)
	assert.Equal(t, "user@gmail.com", cfg.SMTPUsername)
	assert.Equal(t, "secret", cfg.SMTPPassword)
	assert.Equal(t, "alerts@chaos-sec.com", cfg.SMTPFrom)
	assert.Equal(t, "Chaos-Sec Alerts", cfg.SMTPFromName)
	assert.Equal(t, "https://hooks.slack.com/xxx", cfg.SlackWebhookURL)
	assert.Equal(t, "#alerts", cfg.SlackChannel)
	assert.Equal(t, "chaos-bot", cfg.SlackUsername)
	assert.Equal(t, "https://example.com/hook", cfg.WebhookURL)
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.AsyncSend)
	assert.Equal(t, 5, cfg.RetryCount)
	assert.Equal(t, 30, cfg.TimeoutSec)
}

func TestService_SendNotification_AllChannels(t *testing.T) {
	slackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer slackServer.Close()

	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	cfg := &Config{
		SMTPHost:        "smtp.example.com",
		SMTPUsername:    "user",
		SMTPPassword:    "pass",
		SlackWebhookURL: slackServer.URL,
		WebhookURL:      webhookServer.URL,
		Enabled:         true,
	}
	svc := NewService(cfg, nil)

	event := NotificationEvent{
		Type:      "experiment_completed",
		Title:     "Full Test",
		Message:   "All channels",
		Timestamp: time.Now(),
	}

	results := svc.SendNotification(context.Background(), event)

	// All three channels should attempt (email will fail to connect but shouldn't crash)
	assert.Len(t, results, 3)
}
