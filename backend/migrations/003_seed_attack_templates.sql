-- ============================================================================
-- Chaos-Sec: Seed Attack Templates
-- Migration: 003_seed_attack_templates.sql
-- Description: Inserts built-in attack module templates and updates the
--              category constraint to accommodate RBAC and security modules.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- 0. Extend the category CHECK constraint to support new module categories
-- ---------------------------------------------------------------------------

ALTER TABLE attack_templates
    DROP CONSTRAINT chk_attack_templates_category;

ALTER TABLE attack_templates
    ADD CONSTRAINT chk_attack_templates_category
    CHECK (category IN ('network', 'rbac', 'security', 'resource', 'privilege', 'data', 'availability'));

-- ---------------------------------------------------------------------------
-- 1. Pod Egress Test
-- ---------------------------------------------------------------------------

INSERT INTO attack_templates (
    name, slug, category, severity, description,
    mitre_attack_id,
    k8s_manifest,
    parameters,
    prerequisites,
    expected_behavior,
    mitigation,
    is_active, is_system
) VALUES (
    'Pod Egress Test',
    'pod-egress-test',
    'network',
    'medium',
    'Tests whether egress (outbound) network policies are properly enforced by attempting an outbound connection from a pod in the target namespace to a specified destination IP and port. If the connection is blocked, the network policy control is working as expected.',

    'T1071.001',

    '{"template": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: chaos-sec-egress-{{.RunID}}\n  namespace: {{.Namespace}}\n  labels:\n    app: chaos-sec-attacker\n    chaos-sec.io/experiment-id: \"{{.ExperimentID}}\"\n    chaos-sec.io/run-id: \"{{.RunID}}\"\n    chaos-sec.io/module: pod-egress-test\nspec:\n  automountServiceAccountToken: false\n  hostNetwork: false\n  hostPID: false\n  hostIPC: false\n  restartPolicy: Never\n  securityContext:\n    runAsNonRoot: true\n    runAsUser: 65534\n    capabilities:\n      drop: [\"ALL\"]\n    readOnlyRootFilesystem: true\n  containers:\n  - name: attacker\n    image: curlimages/curl:latest\n    command: [\"sh\", \"-c\"]\n    args:\n    - |\n      echo \"Starting egress test to {{.DestinationIP}}:{{.DestinationPort}}...\"\n      for i in $(seq 1 {{.Attempts}}); do\n        echo \"Attempt $i/{{.Attempts}}...\"\n        if curl -s -o /dev/null -w \"%{http_code}\" --connect-timeout {{.TimeoutSeconds}} --max-time {{.TimeoutSeconds}} -{{.Protocol}} {{.DestinationIP}}:{{.DestinationPort}}; then\n          echo \"CONNECTION_SUCCESS: Connected to {{.DestinationIP}}:{{.DestinationPort}}\"\n        else\n          echo \"CONNECTION_BLOCKED: Could not connect to {{.DestinationIP}}:{{.DestinationPort}}\"\n        fi\n        sleep 1\n      done\n    resources:\n      requests: { cpu: 50m, memory: 64Mi }\n      limits: { cpu: 500m, memory: 256Mi }\n  terminationGracePeriodSeconds: 30"}'::jsonb,

    '{
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "type": "object",
      "properties": {
        "target_namespace": {
          "type": "string",
          "description": "Namespace in which to launch the attacker pod"
        },
        "destination_ip": {
          "type": "string",
          "default": "8.8.8.8",
          "description": "IP address to attempt connection to"
        },
        "destination_port": {
          "type": "integer",
          "default": 53,
          "description": "Port number to test on the destination"
        },
        "destination_protocol": {
          "type": "string",
          "enum": ["tcp", "udp", "icmp"],
          "default": "tcp",
          "description": "Protocol to use for the connection test"
        },
        "timeout_seconds": {
          "type": "integer",
          "default": 10,
          "description": "Connection timeout in seconds per attempt"
        },
        "attempts": {
          "type": "integer",
          "default": 3,
          "description": "Number of connection attempts to make"
        }
      },
      "required": ["target_namespace", "destination_ip", "destination_port"]
    }'::jsonb,

    '["Kubernetes cluster with network policies configured","Access to the target namespace","curlimages/curl image available in cluster"]'::jsonb,

    'When egress network policies are correctly configured and enforced, the attacker pod should be unable to establish an outbound connection to the specified destination. The pod logs will show CONNECTION_BLOCKED for each attempt. If no network policies are in place or they are misconfigured, the connection will succeed and the logs will show CONNECTION_SUCCESS.',

    'Implement default-deny egress network policies in all namespaces. Create explicit allow rules for required outbound traffic only. Use Calico or Cilium network policy extensions for fine-grained egress control. Regularly audit network policies to ensure coverage.',

    true, true
);

-- ---------------------------------------------------------------------------
-- 2. Pod Ingress Test
-- ---------------------------------------------------------------------------

INSERT INTO attack_templates (
    name, slug, category, severity, description,
    mitre_attack_id,
    k8s_manifest,
    parameters,
    prerequisites,
    expected_behavior,
    mitigation,
    is_active, is_system
) VALUES (
    'Pod Ingress Test',
    'pod-ingress-test',
    'network',
    'medium',
    'Tests whether ingress (inbound) network policies block unauthorized connections by deploying a target service in the target namespace and attempting to reach it from an attacker pod in a different source namespace. If the connection is blocked, the ingress network policy control is working as expected.',

    'T1071.001',

    '{"template": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: chaos-sec-ingress-target-{{.RunID}}\n  namespace: {{.Namespace}}\n  labels:\n    app: chaos-sec-ingress-target\n    chaos-sec.io/experiment-id: \"{{.ExperimentID}}\"\n    chaos-sec.io/run-id: \"{{.RunID}}\"\n    chaos-sec.io/module: pod-ingress-test\nspec:\n  automountServiceAccountToken: false\n  hostNetwork: false\n  hostPID: false\n  hostIPC: false\n  restartPolicy: Never\n  securityContext:\n    runAsNonRoot: true\n    runAsUser: 65534\n    capabilities:\n      drop: [\"ALL\"]\n    readOnlyRootFilesystem: true\n  containers:\n  - name: server\n    image: curlimages/curl:latest\n    command: [\"sh\", \"-c\"]\n    args:\n    - |\n      echo \"Ingress target server listening on port {{.Port}}...\"\n      while true; do\n        echo -e \"HTTP/1.1 200 OK\\\\r\\\\nContent-Type: text/plain\\\\r\\\\n\\\\r\\\\nchaos-sec-ingress-target\" | nc -l -p {{.Port}} 2>/dev/null || true\n      done\n    resources:\n      requests: { cpu: 50m, memory: 64Mi }\n      limits: { cpu: 500m, memory: 256Mi }\n  terminationGracePeriodSeconds: 30"}'::jsonb,

    '{
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "type": "object",
      "properties": {
        "target_namespace": {
          "type": "string",
          "description": "Namespace where the target service will be deployed"
        },
        "target_service": {
          "type": "string",
          "description": "Name for the target service to deploy and test against"
        },
        "target_port": {
          "type": "integer",
          "default": 80,
          "description": "Port on which the target service listens"
        },
        "source_namespace": {
          "type": "string",
          "default": "default",
          "description": "Namespace from which to launch the attacker pod"
        },
        "timeout_seconds": {
          "type": "integer",
          "default": 10,
          "description": "Connection timeout in seconds per attempt"
        },
        "attempts": {
          "type": "integer",
          "default": 3,
          "description": "Number of connection attempts to make"
        }
      },
      "required": ["target_namespace", "target_service", "target_port"]
    }'::jsonb,

    '["Kubernetes cluster with network policies configured","Access to both target and source namespaces","curlimages/curl image available in cluster"]'::jsonb,

    'When ingress network policies are correctly configured, the attacker pod in the source namespace should be unable to connect to the target service in the target namespace. The attacker pod logs will show CONNECTION_BLOCKED for each attempt. If ingress policies are absent or misconfigured, the connection will succeed and logs will show CONNECTION_SUCCESS.',

    'Implement default-deny ingress network policies in all namespaces. Create explicit allow rules permitting only authorised source pods/namespaces to reach each service. Use namespace selectors and pod selectors in NetworkPolicy to restrict cross-namespace traffic. Consider service mesh solutions (e.g. Istio) for additional L7 policy enforcement.',

    true, true
);

-- ---------------------------------------------------------------------------
-- 3. Network Policy Validation
-- ---------------------------------------------------------------------------

INSERT INTO attack_templates (
    name, slug, category, severity, description,
    mitre_attack_id,
    k8s_manifest,
    parameters,
    prerequisites,
    expected_behavior,
    mitigation,
    is_active, is_system
) VALUES (
    'Network Policy Validation',
    'network-policy-test',
    'network',
    'high',
    'Validates that existing network policies are correctly configured and effectively enforced. Reads network policies from the target namespace, creates test pods that attempt to violate each policy, and reports which policies are effective and which have configuration gaps.',

    'T1562.007',

    '{"template": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: chaos-sec-netpol-{{.Direction}}-{{.RunID}}\n  namespace: {{.Namespace}}\n  labels:\n    app: chaos-sec-netpol-test\n    chaos-sec.io/experiment-id: \"{{.ExperimentID}}\"\n    chaos-sec.io/run-id: \"{{.RunID}}\"\n    chaos-sec.io/module: network-policy-test\nspec:\n  automountServiceAccountToken: false\n  hostNetwork: false\n  hostPID: false\n  hostIPC: false\n  restartPolicy: Never\n  securityContext:\n    runAsNonRoot: true\n    runAsUser: 65534\n    capabilities:\n      drop: [\"ALL\"]\n    readOnlyRootFilesystem: true\n  containers:\n  - name: tester\n    image: curlimages/curl:latest\n    command: [\"sh\", \"-c\"]\n    args:\n    - |\n      echo \"Starting network policy {{.Direction}} test...\"\n      echo \"Target CIDR: {{.TestCIDR}}, Port: {{.TestPort}}\"\n      for i in $(seq 1 {{.Attempts}}); do\n        echo \"Attempt $i/{{.Attempts}}...\"\n        if curl -s -o /dev/null -w \"%{http_code}\" --connect-timeout {{.TimeoutSeconds}} --max-time {{.TimeoutSeconds}} \"https://{{.TestCIDR}}:{{.TestPort}}\" 2>/dev/null; then\n          echo \"CONNECTION_SUCCESS: Connected to {{.TestCIDR}}:{{.TestPort}} via {{.Direction}}\"\n        else\n          echo \"CONNECTION_BLOCKED: Could not connect to {{.TestCIDR}}:{{.TestPort}} via {{.Direction}}\"\n        fi\n        sleep 1\n      done\n    resources:\n      requests: { cpu: 50m, memory: 64Mi }\n      limits: { cpu: 500m, memory: 256Mi }\n  terminationGracePeriodSeconds: 30"}'::jsonb,

    '{
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "type": "object",
      "properties": {
        "target_namespace": {
          "type": "string",
          "description": "Namespace containing the network policies to validate"
        },
        "policy_name": {
          "type": "string",
          "default": "",
          "description": "Specific network policy to test (empty = test all policies in namespace)"
        },
        "test_cidr": {
          "type": "string",
          "default": "0.0.0.0/0",
          "description": "CIDR block to test against egress rules"
        },
        "test_port": {
          "type": "integer",
          "default": 443,
          "description": "Port number to test against policy rules"
        },
        "timeout_seconds": {
          "type": "integer",
          "default": 10,
          "description": "Connection timeout in seconds per attempt"
        },
        "attempts": {
          "type": "integer",
          "default": 3,
          "description": "Number of connection attempts per test"
        }
      },
      "required": ["target_namespace"]
    }'::jsonb,

    '["Kubernetes cluster with NetworkPolicy API enabled","CNI plugin that supports network policies (Calico, Cilium, etc.)","Access to the target namespace","curlimages/curl image available in cluster"]'::jsonb,

    'When network policies are correctly configured and enforced, the test pods should be unable to establish connections that violate the defined policy rules. The validation report will list each policy as EFFECTIVE. If policies have gaps (e.g. missing CIDR blocks, overly permissive port ranges, or empty selectors), the affected policy will be flagged as having a GAP, and the specific traffic that bypassed enforcement will be detailed.',

    'Adopt default-deny policies for both ingress and egress in every namespace. Use specific pod and namespace selectors instead of broad allow rules. Restrict egress CIDR ranges to only required destinations. Limit allowed ports and protocols to the minimum necessary. Validate policies using tools like calicoctl or cilium policy. Implement policy-as-code workflows to review changes before deployment. Regularly run network policy validation tests.',

    true, true
);

-- ---------------------------------------------------------------------------
-- 4. RBAC Privilege Escalation Test
-- ---------------------------------------------------------------------------

INSERT INTO attack_templates (
    name, slug, category, severity, description,
    mitre_attack_id,
    k8s_manifest,
    parameters,
    prerequisites,
    expected_behavior,
    mitigation,
    is_active, is_system
) VALUES (
    'RBAC Privilege Escalation Test',
    'rbac-privilege-test',
    'rbac',
    'high',
    'Tests whether service accounts have excessive permissions by attempting privileged actions (e.g. listing secrets, creating pods, executing into pods) from an attacker pod using the specified service account. If the action is denied, RBAC controls are working correctly; if allowed, a privilege escalation path exists.',

    'T1078.004',

    '{"template": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: chaos-sec-rbac-{{.RunID}}\n  namespace: {{.Namespace}}\n  labels:\n    app: chaos-sec-attacker\n    chaos-sec.io/experiment-id: \"{{.ExperimentID}}\"\n    chaos-sec.io/run-id: \"{{.RunID}}\"\n    chaos-sec.io/module: rbac-privilege-test\nspec:\n  automountServiceAccountToken: true\n  serviceAccountName: {{.ServiceAccount}}\n  hostNetwork: false\n  hostPID: false\n  hostIPC: false\n  restartPolicy: Never\n  securityContext:\n    runAsNonRoot: true\n    runAsUser: 65534\n    capabilities:\n      drop: [\"ALL\"]\n    readOnlyRootFilesystem: true\n  containers:\n  - name: attacker\n    image: bitnami/kubectl:latest\n    command: [\"sh\", \"-c\"]\n    args:\n    - |\n      echo \"Starting RBAC privilege test...\"\n      echo \"Service Account: {{.ServiceAccount}}\"\n      echo \"Namespace: {{.Namespace}}\"\n      echo \"Action: {{.TestAction}}\"\n      for i in $(seq 1 {{.Attempts}}); do\n        echo \"Attempt $i/{{.Attempts}}...\"\n        case \"{{.TestAction}}\" in\n          list-secrets)\n            if kubectl get secrets -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then\n              echo \"ACTION_ALLOWED: Successfully listed secrets in {{.Namespace}}\"\n            else\n              echo \"ACTION_DENIED: Forbidden to list secrets in {{.Namespace}}\"\n            fi\n            ;;\n          create-pods)\n            if kubectl run test-rbac-pod --image=busybox -n {{.Namespace}} --restart=Never --request-timeout={{.TimeoutSeconds}}s 2>&1; then\n              echo \"ACTION_ALLOWED: Successfully created a pod in {{.Namespace}}\"\n              kubectl delete pod test-rbac-pod -n {{.Namespace}} --force --grace-period=0 2>/dev/null || true\n            else\n              echo \"ACTION_DENIED: Forbidden to create pods in {{.Namespace}}\"\n            fi\n            ;;\n          delete-pods)\n            if kubectl delete pods --all -n {{.Namespace}} --dry-run=client --request-timeout={{.TimeoutSeconds}}s 2>&1; then\n              echo \"ACTION_ALLOWED: Dry-run delete pods succeeded (permission exists) in {{.Namespace}}\"\n            else\n              echo \"ACTION_DENIED: Forbidden to delete pods in {{.Namespace}}\"\n            fi\n            ;;\n          list-configmaps)\n            if kubectl get configmaps -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then\n              echo \"ACTION_ALLOWED: Successfully listed configmaps in {{.Namespace}}\"\n            else\n              echo \"ACTION_DENIED: Forbidden to list configmaps in {{.Namespace}}\"\n            fi\n            ;;\n          exec-pods)\n            if kubectl auth can-i create pods/exec -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then\n              echo \"ACTION_ALLOWED: Has exec permissions in {{.Namespace}}\"\n            else\n              echo \"ACTION_DENIED: Forbidden to exec into pods in {{.Namespace}}\"\n            fi\n            ;;\n          *)\n            echo \"UNKNOWN_ACTION: {{.TestAction}} is not a supported test action\"\n            ;;\n        esac\n        sleep 2\n      done\n    resources:\n      requests: { cpu: 50m, memory: 64Mi }\n      limits: { cpu: 500m, memory: 256Mi }\n  terminationGracePeriodSeconds: 30"}'::jsonb,

    '{
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "type": "object",
      "properties": {
        "target_namespace": {
          "type": "string",
          "description": "Namespace in which to launch the attacker pod and test RBAC"
        },
        "service_account": {
          "type": "string",
          "default": "default",
          "description": "Service account to impersonate in the attacker pod"
        },
        "test_action": {
          "type": "string",
          "enum": ["list-secrets", "create-pods", "delete-pods", "list-configmaps", "exec-pods"],
          "description": "Privileged action to attempt via kubectl"
        },
        "timeout_seconds": {
          "type": "integer",
          "default": 30,
          "description": "Timeout in seconds for the kubectl command to complete"
        },
        "attempts": {
          "type": "integer",
          "default": 1,
          "description": "Number of times to attempt the action"
        }
      },
      "required": ["target_namespace", "test_action"]
    }'::jsonb,

    '["Kubernetes cluster with RBAC enabled","Access to the target namespace","bitnami/kubectl image available in cluster","Service account to test must exist in the target namespace"]'::jsonb,

    'When RBAC is correctly configured, the attacker pod using the specified service account should be denied all privileged actions. The pod logs will show ACTION_DENIED for each attempt. If the service account has been granted excessive permissions (e.g. through overly broad ClusterRoleBindings or wildcard rules), the action will succeed and the logs will show ACTION_ALLOWED, indicating a privilege escalation path.',

    'Follow the principle of least privilege for all service accounts. Avoid binding the default service account to any roles. Disable automountServiceAccountToken for pods that do not need API access. Use namespace-scoped Roles instead of ClusterRoles where possible. Audit RBAC bindings regularly with kubectl auth can-i --list. Implement RBAC-as-code with peer review for all policy changes. Remove default permissions and create explicit RoleBindings for each workload.',

    true, true
);

-- ---------------------------------------------------------------------------
-- 5. Secret Access Test
-- ---------------------------------------------------------------------------

INSERT INTO attack_templates (
    name, slug, category, severity, description,
    mitre_attack_id,
    k8s_manifest,
    parameters,
    prerequisites,
    expected_behavior,
    mitigation,
    is_active, is_system
) VALUES (
    'Secret Access Test',
    'secret-access-test',
    'security',
    'high',
    'Tests whether pods can access secrets they should not by deploying an attacker pod that attempts to read secrets via the Kubernetes API and inspect mounted secrets from environment variables and filesystem paths. If access is denied, the security controls are working correctly; if allowed, a secret exposure vulnerability exists.',

    'T1552.007',

    '{"template": "apiVersion: v1\nkind: Pod\nmetadata:\n  name: chaos-sec-secret-api-{{.RunID}}\n  namespace: {{.Namespace}}\n  labels:\n    app: chaos-sec-attacker\n    chaos-sec.io/experiment-id: \"{{.ExperimentID}}\"\n    chaos-sec.io/run-id: \"{{.RunID}}\"\n    chaos-sec.io/module: secret-access-test\nspec:\n  automountServiceAccountToken: true\n  serviceAccountName: default\n  hostNetwork: false\n  hostPID: false\n  hostIPC: false\n  restartPolicy: Never\n  securityContext:\n    runAsNonRoot: true\n    runAsUser: 65534\n    capabilities:\n      drop: [\"ALL\"]\n    readOnlyRootFilesystem: true\n  containers:\n  - name: attacker\n    image: bitnami/kubectl:latest\n    command: [\"sh\", \"-c\"]\n    args:\n    - |\n      echo \"Starting secret access test via Kubernetes API...\"\n      echo \"Namespace: {{.Namespace}}\"\n      if kubectl get secrets -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then\n        echo \"SECRET_LIST_ALLOWED: Successfully listed secrets in {{.Namespace}}\"\n      else\n        echo \"SECRET_LIST_DENIED: Forbidden to list secrets in {{.Namespace}}\"\n      fi\n    resources:\n      requests: { cpu: 50m, memory: 64Mi }\n      limits: { cpu: 500m, memory: 256Mi }\n  terminationGracePeriodSeconds: 30"}'::jsonb,

    '{
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "type": "object",
      "properties": {
        "target_namespace": {
          "type": "string",
          "description": "Namespace in which to launch the attacker pod and test secret access"
        },
        "secret_name": {
          "type": "string",
          "default": "",
          "description": "Specific secret to test access to (empty = attempt to list all secrets)"
        },
        "mount_path": {
          "type": "boolean",
          "default": false,
          "description": "Test whether secrets are accessible via mounted volumes and environment variables"
        },
        "timeout_seconds": {
          "type": "integer",
          "default": 30,
          "description": "Timeout in seconds for each kubectl/API command"
        },
        "attempts": {
          "type": "integer",
          "default": 1,
          "description": "Number of times to attempt each access method"
        }
      },
      "required": ["target_namespace"]
    }'::jsonb,

    '["Kubernetes cluster with RBAC enabled","Access to the target namespace","bitnami/kubectl image available in cluster","curlimages/curl image available in cluster (for mount path test)"]'::jsonb,

    'When security controls are correctly configured, the attacker pod should be unable to list or read secrets via the Kubernetes API, and no sensitive data should be exposed through environment variables or mounted volumes. API access attempts will show SECRET_LIST_DENIED or SECRET_API_DENIED. Mount path tests will show SECRET_ENV_NONE, SECRET_MOUNT_NOT_FOUND, and SECRET_SA_MOUNT_NOT_FOUND. If controls are weak, the attacker will successfully list/read secrets or find sensitive data in mounted paths.',

    'Restrict secret access using RBAC: the default service account should have no secret read permissions. Set automountServiceAccountToken: false on pods that do not need API access. Use sealed secrets or external secret managers (Vault, AWS Secrets Manager, GCP Secret Manager) instead of native Kubernetes secrets. Avoid mounting secrets as environment variables (use volume mounts with readOnly: true). Enable encryption at rest for etcd. Implement audit logging for secret access events. Use Kubernetes Secret RBAC rules to limit which service accounts can read specific secrets.',

    true, true
);

-- ---------------------------------------------------------------------------
-- Index for efficient category lookups on the new categories
-- ---------------------------------------------------------------------------

-- The existing idx_templates_category index already covers the category column.
-- No additional index is needed since the constraint change and index both
-- operate on the same column value.
