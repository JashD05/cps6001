-- ============================================================================
-- Chaos-Sec: Demo Data Seed Migration
-- Migration: 013_seed_demo_reports_and_experiment.up.sql
-- Description: Seeds a completed demo experiment and report so the UI shows real data
-- ============================================================================

DO $$
DECLARE
    v_org_id uuid;
    v_user_id uuid;
    v_cluster_id uuid;
    v_experiment_id uuid;
    v_attack_template_id uuid;
    v_report_id uuid;
BEGIN
    SELECT id INTO v_org_id
    FROM organizations
    ORDER BY created_at
    LIMIT 1;

    SELECT id INTO v_user_id
    FROM users
    ORDER BY created_at
    LIMIT 1;

    SELECT id INTO v_attack_template_id
    FROM attack_templates
    ORDER BY created_at
    LIMIT 1;

    IF v_org_id IS NULL OR v_user_id IS NULL OR v_attack_template_id IS NULL THEN
        RAISE NOTICE 'Skipping demo seed because organizations, users, or attack templates are missing.';
        RETURN;
    END IF;

    SELECT id INTO v_cluster_id
    FROM kubernetes_clusters
    WHERE organization_id = v_org_id AND name = 'Demo Kubernetes Cluster'
    LIMIT 1;

    IF v_cluster_id IS NULL THEN
        v_cluster_id := gen_random_uuid();
        INSERT INTO kubernetes_clusters (
            id, organization_id, name, description, api_endpoint,
            ca_certificate, client_certificate, client_key,
            default_namespace, status, kubernetes_version, node_count,
            last_connected_at, created_at, updated_at
        ) VALUES (
            v_cluster_id,
            v_org_id,
            'Demo Kubernetes Cluster',
            'Seeded cluster for demo experiment output',
            'https://demo.cluster.local',
            'demo-ca-certificate',
            'demo-client-certificate',
            'demo-client-key',
            'chaos-sec',
            'connected',
            '1.29',
            3,
            NOW(),
            NOW(),
            NOW()
        );
    END IF;

    SELECT id INTO v_experiment_id
    FROM experiments
    WHERE organization_id = v_org_id AND name = 'CPU Stress Test Demo'
    LIMIT 1;

    IF v_experiment_id IS NULL THEN
        v_experiment_id := gen_random_uuid();
        INSERT INTO experiments (
            id, organization_id, name, description, status, created_by,
            schedule_cron, auto_cleanup, notification_config, created_at, updated_at
        ) VALUES (
            v_experiment_id,
            v_org_id,
            'CPU Stress Test Demo',
            'Demonstration experiment with completed output data.',
            'archived',
            v_user_id,
            NULL,
            TRUE,
            '{}'::jsonb,
            NOW() - INTERVAL '1 day',
            NOW() - INTERVAL '1 day'
        );
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM experiment_templates et
        WHERE et.experiment_id = v_experiment_id
    ) THEN
        INSERT INTO experiment_templates (
            experiment_id, attack_template_id, order_index, configuration,
            target_namespaces, target_labels, duration_seconds, cleanup_policy,
            siem_validation, enabled
        ) VALUES (
            v_experiment_id,
            v_attack_template_id,
            0,
            '{"severity":"high","duration":"5m","load":"80%"}'::jsonb,
            ARRAY['testing'],
            '{"cluster_id":"demo","namespace":"testing"}'::jsonb,
            300,
            'immediate',
            '{"siemAlertType":"cpu_spike","timeWindowSeconds":300,"expectedAlertCount":2}'::jsonb,
            TRUE
        );
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM experiment_runs er
        WHERE er.experiment_id = v_experiment_id
    ) THEN
        INSERT INTO experiment_runs (
            experiment_id, cluster_id, run_number, status, triggered_by,
            trigger_type, started_at, completed_at, duration_ms,
            result_summary, error_message, cleanup_status, created_at
        ) VALUES (
            v_experiment_id,
            v_cluster_id,
            1,
            'completed',
            v_user_id,
            'manual',
            NOW() - INTERVAL '35 minutes',
            NOW() - INTERVAL '27 minutes',
            480000,
            jsonb_build_object(
                'success', true,
                'score', 95,
                'summary', 'CPU stress test completed successfully with alerts detected and blocked.',
                'details', jsonb_build_array(
                    'All injected pods started successfully.',
                    'CPU usage exceeded the configured threshold.',
                    'SIEM alert generated within the expected window.',
                    'Auto-remediation policy returned the cluster to a healthy state.'
                ),
                'siemValidation', jsonb_build_object(
                    'expectedAlertCount', 2,
                    'receivedAlertCount', 2,
                    'alerts', '[]'::jsonb,
                    'detected', true,
                    'detectionLatencyMs', 1200,
                    'coverage', 100,
                    'details', jsonb_build_array(
                        'Detected high CPU activity on demo namespace.',
                        'Alert correlated to the completed run.'
                    )
                ),
                'startedAt', to_char(NOW() - INTERVAL '35 minutes', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
                'completedAt', to_char(NOW() - INTERVAL '27 minutes', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
                'duration', 480000
            ),
            NULL,
            'completed',
            NOW() - INTERVAL '27 minutes'
        );
    END IF;

    SELECT id INTO v_report_id
    FROM reports
    WHERE organization_id = v_org_id AND title = 'CPU Stress Test Demo Report'
    LIMIT 1;

    IF v_report_id IS NULL THEN
        v_report_id := gen_random_uuid();
        INSERT INTO reports (
            id, organization_id, title, type, format, description,
            experiment_ids, date_range_from, date_range_to, status,
            download_url, file_size, generated_by, created_at, updated_at
        ) VALUES (
            v_report_id,
            v_org_id,
            'CPU Stress Test Demo Report',
            'experiment',
            'pdf',
            'Completed experiment output with validation summary.',
            to_jsonb(ARRAY[v_experiment_id::text]),
            NOW() - INTERVAL '1 day',
            NOW(),
            'ready',
            '/reports/' || v_report_id::text || '/download',
            2456789,
            v_user_id,
            NOW(),
            NOW()
        );
    END IF;
END $$;
