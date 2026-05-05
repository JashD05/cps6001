# =============================================================================
# Chaos-Sec Platform – AWS EKS Infrastructure
# =============================================================================
# Production-ready Terraform configuration for deploying the Chaos-Sec
# platform on AWS EKS. This module provisions:
#
#   - VPC with public and private subnets across multiple AZs
#   - EKS cluster (Kubernetes v1.28) with managed node groups
#   - IAM roles and policies for EKS, node groups, and Chaos-Sec services
#   - Security groups for cluster, node, and database communication
#   - EBS encryption, CloudWatch logging, and audit trail
#
# Usage:
#   terraform init
#   terraform plan
#   terraform apply
#
# Requirements:
#   - Terraform >= 1.5
#   - AWS CLI v2 configured with appropriate credentials
#   - kubectl for cluster access after deployment
# =============================================================================

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.26"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.12"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }

  # ──────────────────────────────────────────────
  # Remote State Backend
  # ──────────────────────────────────────────────
  # Uncomment and configure for production state storage.
  # S3 backend provides state locking via DynamoDB and
  # encryption at rest.
  # backend "s3" {
  #   bucket         = "chaos-sec-terraform-state"
  #   key            = "eks/terraform.tfstate"
  #   region         = "eu-west-2"
  #   encrypt        = true
  #   dynamodb_table = "chaos-sec-terraform-locks"
  #   kms_key_id     = "alias/chaos-sec-terraform"
  #   role_arn       = ""
  # }
}

# =============================================================================
# Provider Configuration
# =============================================================================

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "Chaos-Sec"
      Environment = var.environment
      ManagedBy   = "Terraform"
      Owner       = var.owner
      CostCenter  = var.cost_center
    }
  }
}

# Kubernetes provider – configured after EKS cluster is created
provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
  token                  = data.aws_eks_cluster_auth.cluster.token
}

# Helm provider – for installing ingress controller and cert-manager
provider "helm" {
  kubernetes {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
    token                  = data.aws_eks_cluster_auth.cluster.token
  }
}

# =============================================================================
# Data Sources
# =============================================================================

data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_eks_cluster_auth" "cluster" {
  name = module.eks.cluster_name
}

# =============================================================================
# Locals
# =============================================================================

locals {
  name_prefix = "${var.project_name}-${var.environment}"

  # Availability zones – use 3 for production HA
  azs = slice(data.aws_availability_zones.available.names, 0, var.az_count)

  # Common tags applied to all resources
  common_tags = {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "Terraform"
    Owner       = var.owner
    CostCenter  = var.cost_center
  }

  # Kubernetes resource tags for EKS
  k8s_tags = {
    "kubernetes.io/cluster/${module.eks.cluster_name}" = "owned"
  }
}

# =============================================================================
# VPC Module
# =============================================================================

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.8"

  name = "${local.name_prefix}-vpc"
  cidr = var.vpc_cidr

  azs = local.azs

  # ──────────────────────────────────────────────
  # Public Subnets
  # ──────────────────────────────────────────────
  # Used for ALB/NLB, NAT Gateway, and bastion hosts.
  # EKS will tag these for external load balancer placement.
  public_subnets = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 4, i)]

  # ──────────────────────────────────────────────
  # Private Subnets (Application)
  # ──────────────────────────────────────────────
  # Used for EKS worker nodes and application pods.
  # EKS will tag these for internal load balancer placement.
  private_subnets = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 4, i + var.az_count)]

  # ──────────────────────────────────────────────
  # Database Subnets (Isolated)
  # ──────────────────────────────────────────────
  # Used for RDS/PostgreSQL and ElastiCache/Redis.
  # No internet access; only reachable from private subnets.
  database_subnets = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 4, i + 2*var.az_count)]

  # Enable DNS support for EKS
  enable_dns_support   = true
  enable_dns_hostnames = true

  # NAT Gateway for private subnet egress
  enable_nat_gateway   = true
  single_nat_gateway   = var.environment != "production"
  one_nat_gateway_per_az = var.environment == "production"

  # VPC Flow Logs for security audit
  enable_vpc_flow_log = true
  vpc_flow_log_tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-flow-log"
  })

  # EKS-required tags for Load Balancer Controller auto-discovery
  public_subnet_tags = merge(local.k8s_tags, {
    "kubernetes.io/role/elb" = "1"
  })

  private_subnet_tags = merge(local.k8s_tags, {
    "kubernetes.io/role/internal-elb" = "1"
  })

  database_subnet_tags = merge(local.common_tags, {
    Tier = "database"
  })

  tags = local.common_tags
}

# =============================================================================
# EKS Cluster Module
# =============================================================================

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.33"

  cluster_name    = "${local.name_prefix}-cluster"
  cluster_version = var.kubernetes_version

  # ──────────────────────────────────────────────
  # Cluster Networking
  # ──────────────────────────────────────────────
  cluster_endpoint_private_access = true
  cluster_endpoint_public_access  = true

  # Restrict public access to specific CIDRs for security
  cluster_endpoint_public_access_cidrs = var.allowed_public_access_cidrs

  # Use VPC subnets for EKS
  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnet_ids

  # ──────────────────────────────────────────────
  # Cluster Authentication
  # ──────────────────────────────────────────────
  # IAM roles that can access the Kubernetes API
  cluster_admin_roles = [
    "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/${var.admin_role_name}"
  ]

  # Enable IAM Identity Provider for Kubernetes RBAC
  cluster_identity_providers = {
    main = {
      client_id                     = "sts.amazonaws.com"
      identity_provider_config_name = "chaos-sec-oidc"
    }
  }

  # ──────────────────────────────────────────────
  # Cluster Addons
  # ──────────────────────────────────────────────
  cluster_addons = {
    coredns = {
      most_recent = true
      configuration_values = jsonencode({
        tolerations = [{
          key      = "CriticalAddonsOnly"
          operator = "Exists"
        }]
      })
    }
    kube-proxy = {
      most_recent = true
    }
    vpc-cni = {
      most_recent    = true
      before_compute = true
      configuration_values = jsonencode({
        env = {
          ENABLE_PREFIX_DELEGATION = "true"
          WARM_PREFIX_TARGET       = "1"
        }
      })
    }
    aws-ebs-csi-driver = {
      most_recent              = true
      service_account_role_arn = module.ebs_csi_irsa.iam_role_arn
    }
  }

  # ──────────────────────────────────────────────
  # Cluster Encryption
  # ──────────────────────────────────────────────
  # Encrypt Kubernetes secrets at rest with a KMS key
  create_kms_key = true
  kms_key_deletion_window_in_days = 7
  cluster_encryption_config = {
    resources = ["secrets"]
  }

  # ──────────────────────────────────────────────
  # Cluster Logging
  # ──────────────────────────────────────────────
  # Enable CloudWatch logging for audit and security
  cluster_enabled_log_types = [
    "api",
    "audit",
    "authenticator",
    "controllerManager",
    "scheduler"
  ]

  # ──────────────────────────────────────────────
  # Security Groups
  # ──────────────────────────────────────────────
  # Additional security group rules for cluster communication
  create_cluster_security_group = true

  cluster_security_group_additional_rules = {
    # Allow ingress from VPC for internal service communication
    ingress_vpc = {
      description = "Allow ingress from VPC CIDR"
      type        = "ingress"
      from_port   = 0
      to_port     = 0
      protocol    = "-1"
      cidr_blocks = [var.vpc_cidr]
    }
  }

  # ──────────────────────────────────────────────
  # Node Groups
  # ──────────────────────────────────────────────
  # Defined in the eks_managed_node_groups variable below.
  # Using managed node groups for simplified lifecycle management.

  eks_managed_node_groups = {
    # ──────────────────────────────────────────
    # Application Node Group
    # ──────────────────────────────────────────
    # General-purpose nodes for backend and frontend workloads.
    # Uses ARM64 Graviton instances for cost efficiency.
    app = {
      name           = "${local.name_prefix}-app"
      description    = "Application node group for Chaos-Sec backend and frontend"
      ami_type       = var.node_group_ami_type
      instance_types = var.app_node_instance_types

      min_size     = var.app_node_min_size
      max_size     = var.app_node_max_size
      desired_size = var.app_node_desired_size

      # Use private subnets
      subnet_ids = module.vpc.private_subnet_ids

      # EBS volume configuration
      block_device_mappings = {
        xvda = {
          device_name = "/dev/xvda"
          ebs = {
            volume_type = "gp3"
            volume_size = var.app_node_disk_size
            encrypted   = true
            kms_key_id  = module.eks.kms_key_arn
          }
        }
      }

      # Kubernetes labels for pod scheduling
      labels = {
        "node-group"                     = "app"
        "chaos-sec.io/role"              = "application"
        "chaos-sec.io/chaos-target"      = "false"
      }

      # Kubernetes taints and tolerations
      taints = {}

      # Security group rules for node communication
      vpc_security_group_ids = [
        aws_security_group.eks_nodes.id,
        aws_security_group.eks_app.id
      ]

      tags = merge(local.common_tags, {
        Name = "${local.name_prefix}-app-node-group"
        Role = "application"
      })
    }

    # ──────────────────────────────────────────
    # Data Node Group
    # ──────────────────────────────────────────
    # Higher-spec nodes for database and cache workloads
    # (PostgreSQL, Redis). These need more memory and I/O.
    data = {
      name           = "${local.name_prefix}-data"
      description    = "Data node group for Chaos-Sec PostgreSQL and Redis"
      ami_type       = var.node_group_ami_type
      instance_types = var.data_node_instance_types

      min_size     = var.data_node_min_size
      max_size     = var.data_node_max_size
      desired_size = var.data_node_desired_size

      subnet_ids = module.vpc.private_subnet_ids

      block_device_mappings = {
        xvda = {
          device_name = "/dev/xvda"
          ebs = {
            volume_type = "gp3"
            volume_size = var.data_node_disk_size
            encrypted   = true
            kms_key_id  = module.eks.kms_key_arn
            iops        = 6000
            throughput  = 250
          }
        }
      }

      labels = {
        "node-group"                     = "data"
        "chaos-sec.io/role"              = "data"
        "chaos-sec.io/chaos-target"      = "false"
      }

      # Taint to prevent non-data pods from scheduling here
      taints = {
        dedicated = {
          key    = "chaos-sec.io/dedicated"
          value  = "data"
          effect = "NO_SCHEDULE"
        }
      }

      vpc_security_group_ids = [
        aws_security_group.eks_nodes.id,
        aws_security_group.eks_data.id
      ]

      tags = merge(local.common_tags, {
        Name = "${local.name_prefix}-data-node-group"
        Role = "data"
      })
    }

    # ──────────────────────────────────────────
    # Chaos Experiment Node Group
    # ──────────────────────────────────────────
    # Dedicated nodes for running chaos experiments.
    # These nodes are expendable and may be terminated
    # during fault injection testing.
    chaos = {
      name           = "${local.name_prefix}-chaos"
      description    = "Chaos experiment node group – targets for fault injection"
      ami_type       = var.node_group_ami_type
      instance_types = var.chaos_node_instance_types

      min_size     = var.chaos_node_min_size
      max_size     = var.chaos_node_max_size
      desired_size = var.chaos_node_desired_size

      subnet_ids = module.vpc.private_subnet_ids

      block_device_mappings = {
        xvda = {
          device_name = "/dev/xvda"
          ebs = {
            volume_type = "gp3"
            volume_size = var.chaos_node_disk_size
            encrypted   = true
            kms_key_id  = module.eks.kms_key_arn
          }
        }
      }

      labels = {
        "node-group"                     = "chaos"
        "chaos-sec.io/role"              = "chaos-target"
        "chaos-sec.io/chaos-target"      = "true"
      }

      # Allow all chaos experiment pods on these nodes
      taints = {}

      vpc_security_group_ids = [
        aws_security_group.eks_nodes.id
      ]

      tags = merge(local.common_tags, {
        Name = "${local.name_prefix}-chaos-node-group"
        Role = "chaos-experiment"
      })
    }
  }

  # ──────────────────────────────────────────────
  # IRSA (IAM Roles for Service Accounts)
  # ──────────────────────────────────────────────
  # Required for EBS CSI driver and Chaos-Sec services
  enable_irsa = true

  tags = local.common_tags
}

# =============================================================================
# EBS CSI Driver IAM Role
# =============================================================================

module "ebs_csi_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.40"

  role_name             = "${local.name_prefix}-ebs-csi-driver"
  attach_ebs_csi_policy = true

  oidc_providers = {
    main = {
      provider_arn = module.eks.oidc_provider_arn
      namespace    = "kube-system"
      service_account = "ebs-csi-controller-sa"
    }
  }

  tags = local.common_tags
}

# =============================================================================
# Chaos-Sec Backend Service Account IAM Role
# =============================================================================

module "backend_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.40"

  role_name = "${local.name_prefix}-backend-sa"

  # Allow the backend to manage EKS resources for chaos experiments
  role_policy_arns = {
    eks_management = aws_iam_policy.backend_eks_management.arn
    s3_access      = aws_iam_policy.backend_s3_access.arn
  }

  oidc_providers = {
    main = {
      provider_arn = module.eks.oidc_provider_arn
      namespace    = "chaos-sec"
      service_account = "chaos-sec-backend"
    }
  }

  tags = local.common_tags
}

# =============================================================================
# IAM Policies
# =============================================================================

# ──────────────────────────────────────────────
# Backend EKS Management Policy
# ──────────────────────────────────────────────
# Allows the backend to create, delete, and manage Kubernetes
# resources for chaos experiment orchestration.
resource "aws_iam_policy" "backend_eks_management" {
  name        = "${local.name_prefix}-backend-eks-management"
  description = "IAM policy for Chaos-Sec backend to manage EKS resources for chaos experiments"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "EKSDescribeCluster"
        Effect = "Allow"
        Action = [
          "eks:DescribeCluster",
          "eks:ListClusters"
        ]
        Resource = [module.eks.cluster_arn]
      },
      {
        Sid    = "EC2DescribeForChaos"
        Effect = "Allow"
        Action = [
          "ec2:DescribeInstances",
          "ec2:DescribeInstanceStatus",
          "ec2:DescribeAvailabilityZones"
        ]
        Resource = ["*"]
      },
      {
        Sid    = "CloudWatchMetrics"
        Effect = "Allow"
        Action = [
          "cloudwatch:GetMetricData",
          "cloudwatch:GetMetricStatistics",
          "cloudwatch:ListMetrics"
        ]
        Resource = ["*"]
      }
    ]
  })

  tags = local.common_tags
}

# ──────────────────────────────────────────────
# Backend S3 Access Policy
# ──────────────────────────────────────────────
# Allows the backend to read/write to S3 buckets for
# experiment results, logs, and report storage.
resource "aws_iam_policy" "backend_s3_access" {
  name        = "${local.name_prefix}-backend-s3-access"
  description = "IAM policy for Chaos-Sec backend to access S3 storage"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "S3BucketAccess"
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:ListBucket",
          "s3:GetBucketLocation"
        ]
        Resource = [
          aws_s3_bucket.chaos_sec_data.arn,
          "${aws_s3_bucket.chaos_sec_data.arn}/*"
        ]
      }
    ]
  })

  tags = local.common_tags
}

# =============================================================================
# S3 Bucket for Experiment Data
# =============================================================================

resource "aws_s3_bucket" "chaos_sec_data" {
  bucket = "${local.name_prefix}-data-${data.aws_caller_identity.current.account_id}"

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-data-bucket"
  })
}

resource "aws_s3_bucket_versioning" "chaos_sec_data" {
  bucket = aws_s3_bucket.chaos_sec_data.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "chaos_sec_data" {
  bucket = aws_s3_bucket.chaos_sec_data.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "aws:kms"
      kms_master_key_id = module.eks.kms_key_arn
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_public_access_block" "chaos_sec_data" {
  bucket = aws_s3_bucket.chaos_sec_data.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "chaos_sec_data" {
  bucket = aws_s3_bucket.chaos_sec_data.id

  rule {
    id     = "transition-to-ia"
    status = "Enabled"

    transition {
      days          = 90
      storage_class = "STANDARD_IA"
    }

    transition {
      days          = 180
      storage_class = "GLACIER"
    }

    expiration {
      days = 365
    }

    noncurrent_version_transition {
      noncurrent_days = 30
      storage_class   = "STANDARD_IA"
    }

    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }
}

# =============================================================================
# Security Groups
# =============================================================================

# ──────────────────────────────────────────────
# EKS Cluster Security Group
# ──────────────────────────────────────────────
# Controls traffic to/from the EKS control plane.
resource "aws_security_group" "eks_cluster" {
  name        = "${local.name_prefix}-cluster"
  description = "Security group for Chaos-Sec EKS cluster control plane"
  vpc_id      = module.vpc.vpc_id

  # Allow inbound API access from VPC
  ingress {
    description = "Allow Kubernetes API access from VPC"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr]
  }

  # Allow inbound from allowed CIDRs (e.g., office VPN)
  ingress {
    description = "Allow Kubernetes API access from trusted networks"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = var.allowed_public_access_cidrs
  }

  # Allow all outbound traffic
  egress {
    description = "Allow all outbound traffic from cluster"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-cluster-sg"
  })
}

# ──────────────────────────────────────────────
# EKS Nodes Security Group
# ──────────────────────────────────────────────
# Base security group for all EKS worker nodes.
# Controls inter-node communication and cluster access.
resource "aws_security_group" "eks_nodes" {
  name        = "${local.name_prefix}-nodes"
  description = "Security group for Chaos-Sec EKS worker nodes"
  vpc_id      = module.vpc.vpc_id

  # Allow all traffic between nodes within the security group
  ingress {
    description = "Allow inter-node communication"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    self        = true
  }

  # Allow inbound from cluster control plane
  ingress {
    description     = "Allow inbound from EKS cluster control plane"
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    security_groups = [aws_security_group.eks_cluster.id]
  }

  # Allow all outbound traffic
  egress {
    description = "Allow all outbound traffic from nodes"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name                                        = "${local.name_prefix}-nodes-sg"
    "kubernetes.io/cluster/${module.eks.cluster_name}" = "owned"
  })
}

# ──────────────────────────────────────────────
# Application Node Security Group
# ──────────────────────────────────────────────
# Additional security group for application (backend/frontend) nodes.
resource "aws_security_group" "eks_app" {
  name        = "${local.name_prefix}-app"
  description = "Security group for Chaos-Sec application workloads"
  vpc_id      = module.vpc.vpc_id

  # Allow HTTP/HTTPS inbound from load balancer
  ingress {
    description     = "Allow HTTP from load balancer"
    from_port       = 80
    to_port         = 80
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_nodes.id]
  }

  ingress {
    description     = "Allow HTTPS from load balancer"
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_nodes.id]
  }

  # Backend API port
  ingress {
    description     = "Allow backend API traffic"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_nodes.id]
  }

  # Backend metrics port
  ingress {
    description     = "Allow backend metrics traffic"
    from_port       = 9090
    to_port         = 9090
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_nodes.id]
  }

  # Allow all outbound traffic
  egress {
    description = "Allow all outbound traffic from app nodes"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-app-sg"
  })
}

# ──────────────────────────────────────────────
# Data Node Security Group
# ──────────────────────────────────────────────
# Additional security group for data (PostgreSQL/Redis) nodes.
resource "aws_security_group" "eks_data" {
  name        = "${local.name_prefix}-data"
  description = "Security group for Chaos-Sec data workloads (PostgreSQL, Redis)"
  vpc_id      = module.vpc.vpc_id

  # PostgreSQL port – only from app nodes
  ingress {
    description     = "Allow PostgreSQL access from app nodes"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_app.id, aws_security_group.eks_nodes.id]
  }

  # Redis port – only from app nodes
  ingress {
    description     = "Allow Redis access from app nodes"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_app.id, aws_security_group.eks_nodes.id]
  }

  # Allow all outbound traffic
  egress {
    description = "Allow all outbound traffic from data nodes"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-data-sg"
  })
}

# =============================================================================
# RDS PostgreSQL
# =============================================================================

# ──────────────────────────────────────────────
# RDS Security Group
# ──────────────────────────────────────────────
# Controls traffic to the RDS PostgreSQL instance.
# Only EKS pods in the app and node security groups can connect.
resource "aws_security_group" "rds" {
  name        = "${local.name_prefix}-rds"
  description = "Security group for Chaos-Sec RDS PostgreSQL"
  vpc_id      = module.vpc.vpc_id

  # PostgreSQL port – only from EKS pods
  ingress {
    description     = "Allow PostgreSQL access from EKS app nodes"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_app.id, aws_security_group.eks_nodes.id]
  }

  # Allow all outbound traffic
  egress {
    description = "Allow all outbound traffic from RDS"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-rds-sg"
  })
}

# ──────────────────────────────────────────────
# DB Subnet Group
# ──────────────────────────────────────────────
# Uses the isolated database subnets from the VPC module.
resource "aws_db_subnet_group" "chaos_sec" {
  name       = "${local.name_prefix}-db"
  subnet_ids = module.vpc.database_subnet_ids

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-db-subnet-group"
  })
}

# ──────────────────────────────────────────────
# RDS PostgreSQL Instance
# ──────────────────────────────────────────────
# Production-grade PostgreSQL 15 on RDS with Multi-AZ,
# encrypted storage, and automated backups.
resource "aws_db_instance" "chaos_sec_postgres" {
  identifier = "${local.name_prefix}-postgres"

  # Engine configuration
  engine            = "postgres"
  engine_version    = "15"
  instance_class    = var.db_instance_class

  # Storage configuration
  allocated_storage     = 20
  max_allocated_storage = 100
  storage_type         = "gp3"
  storage_encrypted    = true
  kms_key_id           = module.eks.kms_key_arn

  # Database credentials
  db_name  = "chaos_sec"
  username = "chaos_sec_admin"
  password = var.db_password
  port     = 5432

  # Multi-AZ for production
  multi_az               = var.environment == "production"
  availability_zone      = var.environment == "production" ? null : local.azs[0]

  # Network configuration
  db_subnet_group_name   = aws_db_subnet_group.chaos_sec.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  # Backup configuration
  backup_retention_period = var.environment == "production" ? 7 : 1
  backup_window           = "03:00-04:00"

  # Maintenance
  maintenance_window         = "Mon:04:00-Mon:05:00"
  auto_minor_version_upgrade = true

  # Performance Insights (production only)
  performance_insights_enabled          = var.environment == "production"
  performance_insights_retention_period = var.environment == "production" ? 7 : 0

  # Logging
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]

  # Deletion protection for production
  deletion_protection       = var.environment == "production"
  skip_final_snapshot       = var.environment != "production"
  final_snapshot_identifier = "${local.name_prefix}-postgres-final-snapshot"

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-postgres"
  })
}

# =============================================================================
# ElastiCache Redis
# =============================================================================

# ──────────────────────────────────────────────
# ElastiCache Security Group
# ──────────────────────────────────────────────
# Controls traffic to the ElastiCache Redis cluster.
# Only EKS pods in the app and node security groups can connect.
resource "aws_security_group" "elasticache" {
  name        = "${local.name_prefix}-elasticache"
  description = "Security group for Chaos-Sec ElastiCache Redis"
  vpc_id      = module.vpc.vpc_id

  # Redis port – only from EKS pods
  ingress {
    description     = "Allow Redis access from EKS app nodes"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.eks_app.id, aws_security_group.eks_nodes.id]
  }

  # Allow all outbound traffic
  egress {
    description = "Allow all outbound traffic from ElastiCache"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-elasticache-sg"
  })
}

# ──────────────────────────────────────────────
# ElastiCache Subnet Group
# ──────────────────────────────────────────────
resource "aws_elasticache_subnet_group" "chaos_sec" {
  name       = "${local.name_prefix}-cache"
  subnet_ids = module.vpc.database_subnet_ids

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-cache-subnet-group"
  })
}

# ──────────────────────────────────────────────
# ElastiCache Redis Replication Group
# ──────────────────────────────────────────────
# Managed Redis with encryption at rest and in transit.
resource "aws_elasticache_replication_group" "chaos_sec_redis" {
  replication_group_id = "${local.name_prefix}-redis"
  description          = "Chaos-Sec Redis replication group"

  # Engine configuration
  engine         = "redis"
  engine_version = "7.1"
  node_type      = var.redis_node_type

  # Cluster configuration
  num_cache_clusters = 1
  num_node_groups   = 1

  # Network configuration
  subnet_group_name  = aws_elasticache_subnet_group.chaos_sec.name
  security_group_ids = [aws_security_group.elasticache.id]

  # Authentication
  auth_token                 = var.redis_password
  transit_encryption_enabled = true
  at_rest_encryption_enabled = true

  # Parameter group
  parameter_group_name = "default.redis7"

  # Automatic failover (only for multi-node groups)
  automatic_failover_enabled = false

  # Maintenance
  maintenance_window = "sun:05:00-sun:06:00"

  # Snapshot configuration
  snapshot_retention_limit = var.environment == "production" ? 7 : 1
  snapshot_window          = "04:00-05:00"

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-redis"
  })
}

# =============================================================================
# Secrets Manager
# =============================================================================

# ──────────────────────────────────────────────
# Database Credentials Secret
# ──────────────────────────────────────────────
# Stores RDS PostgreSQL credentials for retrieval by EKS pods.
resource "aws_secretsmanager_secret" "db_credentials" {
  name                    = "${local.name_prefix}-db-credentials"
  description             = "Chaos-Sec PostgreSQL database credentials"
  recovery_window_in_days = 7

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-db-credentials-secret"
  })
}

resource "aws_secretsmanager_secret_version" "db_credentials" {
  secret_id = aws_secretsmanager_secret.db_credentials.id
  secret_string = jsonencode({
    username = "chaos_sec_admin"
    password = var.db_password
    host     = aws_db_instance.chaos_sec_postgres.address
    port     = aws_db_instance.chaos_sec_postgres.port
    database = "chaos_sec"
    url      = "postgresql://chaos_sec_admin:${var.db_password}@${aws_db_instance.chaos_sec_postgres.address}:${aws_db_instance.chaos_sec_postgres.port}/chaos_sec"
  })
}

# ──────────────────────────────────────────────
# JWT Secret
# ──────────────────────────────────────────────
resource "aws_secretsmanager_secret" "jwt_secret" {
  name                    = "${local.name_prefix}-jwt-secret"
  description             = "Chaos-Sec JWT signing secret"
  recovery_window_in_days = 7

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-jwt-secret-secret"
  })
}

resource "aws_secretsmanager_secret_version" "jwt_secret" {
  secret_id     = aws_secretsmanager_secret.jwt_secret.id
  secret_string = jsonencode({
    jwt_secret = var.jwt_secret
  })
}

# ──────────────────────────────────────────────
# Redis Credentials Secret
# ──────────────────────────────────────────────
resource "aws_secretsmanager_secret" "redis_credentials" {
  name                    = "${local.name_prefix}-redis-credentials"
  description             = "Chaos-Sec Redis credentials"
  recovery_window_in_days = 7

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-redis-credentials-secret"
  })
}

resource "aws_secretsmanager_secret_version" "redis_credentials" {
  secret_id = aws_secretsmanager_secret.redis_credentials.id
  secret_string = jsonencode({
    host     = aws_elasticache_replication_group.chaos_sec_redis.primary_endpoint_address
    port     = aws_elasticache_replication_group.chaos_sec_redis.port
    password = var.redis_password
    url      = "redis://:${var.redis_password}@${aws_elasticache_replication_group.chaos_sec_redis.primary_endpoint_address}:${aws_elasticache_replication_group.chaos_sec_redis.port}"
  })
}

# =============================================================================
# Kubernetes Resources (applied after EKS cluster is ready)
# =============================================================================

# ──────────────────────────────────────────────
# Chaos-Sec Namespace
# ──────────────────────────────────────────────
resource "kubernetes_namespace" "chaos_sec" {
  metadata {
    name = "chaos-sec"
    labels = {
      "app.kubernetes.io/name"    = "chaos-sec"
      "app.kubernetes.io/part-of" = "chaos-sec"
    }
    annotations = {
      "description" = "Chaos-Sec security chaos engineering platform – production namespace"
    }
  }

  depends_on = [module.eks]
}

# ──────────────────────────────────────────────
# Chaos-Sec Backend Service Account
# ──────────────────────────────────────────────
resource "kubernetes_service_account" "backend" {
  metadata {
    name      = "chaos-sec-backend"
    namespace = kubernetes_namespace.chaos_sec.metadata[0].name
    annotations = {
      "eks.amazonaws.com/role-arn" = module.backend_irsa.iam_role_arn
    }
    labels = {
      "app.kubernetes.io/name" = "chaos-sec-backend"
      "app.kubernetes.io/part-of" = "chaos-sec"
    }
  }

  depends_on = [module.eks, kubernetes_namespace.chaos_sec]
}

# ──────────────────────────────────────────────
# Chaos-Sec Frontend Service Account
# ──────────────────────────────────────────────
resource "kubernetes_service_account" "frontend" {
  metadata {
    name      = "chaos-sec-frontend"
    namespace = kubernetes_namespace.chaos_sec.metadata[0].name
    labels = {
      "app.kubernetes.io/name" = "chaos-sec-frontend"
      "app.kubernetes.io/part-of" = "chaos-sec"
    }
  }

  depends_on = [module.eks, kubernetes_namespace.chaos_sec]
}

# ──────────────────────────────────────────────
# Chaos-Sec PostgreSQL Service Account
# ──────────────────────────────────────────────
resource "kubernetes_service_account" "postgres" {
  metadata {
    name      = "chaos-sec-postgres"
    namespace = kubernetes_namespace.chaos_sec.metadata[0].name
    labels = {
      "app.kubernetes.io/name" = "chaos-sec-postgres"
      "app.kubernetes.io/part-of" = "chaos-sec"
    }
  }

  depends_on = [module.eks, kubernetes_namespace.chaos_sec]
}

# ──────────────────────────────────────────────
# Chaos-Sec Redis Service Account
# ──────────────────────────────────────────────
resource "kubernetes_service_account" "redis" {
  metadata {
    name      = "chaos-sec-redis"
    namespace = kubernetes_namespace.chaos_sec.metadata[0].name
    labels = {
      "app.kubernetes.io/name" = "chaos-sec-redis"
      "app.kubernetes.io/part-of" = "chaos-sec"
    }
  }

  depends_on = [module.eks, kubernetes_namespace.chaos_sec]
}

# =============================================================================
# Helm Releases
# =============================================================================

# ──────────────────────────────────────────────
# AWS Load Balancer Controller
# ──────────────────────────────────────────────
# Required for creating AWS ALB/NLB for Kubernetes Ingress
resource "helm_release" "aws_load_balancer_controller" {
  name       = "aws-load-balancer-controller"
  namespace  = "kube-system"
  repository  = "https://aws.github.io/eks-charts"
  chart      = "aws-load-balancer-controller"
  version    = "1.7.1"

  set {
    name  = "clusterName"
    value = module.eks.cluster_name
  }

  set {
    name  = "serviceAccount.create"
    value = "true"
  }

  set {
    name  = "serviceAccount.name"
    value = "aws-load-balancer-controller"
  }

  set {
    name  = "serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn"
    value = module.load_balancer_controller_irsa.iam_role_arn
  }

  set {
    name  = "region"
    value = var.aws_region
  }

  set {
    name  = "vpcId"
    value = module.vpc.vpc_id
  }

  set {
    name  = "replicaCount"
    value = "2"
  }

  depends_on = [module.eks]

  tags = local.common_tags
}

# IAM role for AWS Load Balancer Controller
module "load_balancer_controller_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.40"

  role_name = "${local.name_prefix}-lb-controller"

  attach_load_balancer_controller_policy = true

  oidc_providers = {
    main = {
      provider_arn     = module.eks.oidc_provider_arn
      namespace        = "kube-system"
      service_account  = "aws-load-balancer-controller"
    }
  }

  tags = local.common_tags
}

# ──────────────────────────────────────────────
# NGINX Ingress Controller
# ──────────────────────────────────────────────
# Deployed via Helm for production-grade ingress routing
resource "helm_release" "ingress_nginx" {
  name       = "ingress-nginx"
  namespace  = "ingress-nginx"
  create_namespace = true

  repository  = "https://kubernetes.github.io/ingress-nginx"
  chart      = "ingress-nginx"
  version    = "4.9.1"

  set {
    name  = "controller.replicaCount"
    value = "2"
  }

  set {
    name  = "controller.service.type"
    value = "LoadBalancer"
  }

  set {
    name  = "controller.service.annotations.service\\.beta\\.kubernetes\\.io/aws-load-balancer-type"
    value = "nlb"
  }

  set {
    name  = "controller.service.annotations.service\\.beta\\.kubernetes\\.io/aws-load-balancer-internal"
    value = "false"
  }

  set {
    name  = "controller.config.proxy-body-size"
    value = "10m"
  }

  set {
    name  = "controller.config.proxy-read-timeout"
    value = "3600"
  }

  set {
    name  = "controller.config.proxy-send-timeout"
    value = "3600"
  }

  set {
    name  = "controller.config.enable-websockets"
    value = "true"
  }

  set {
    name  = "controller.config.use-forwarded-headers"
    value = "true"
  }

  set {
    name  = "controller.config.compute-full-forwarded-for"
    value = "true"
  }

  set {
    name  = "controller.metrics.enabled"
    value = "true"
  }

  set {
    name  = "defaultBackend.enabled"
    value = "false"
  }

  depends_on = [module.eks]

  tags = local.common_tags
}

# ──────────────────────────────────────────────
# cert-manager (Optional – uncomment for automatic TLS)
# ──────────────────────────────────────────────
# resource "helm_release" "cert_manager" {
#   name       = "cert-manager"
#   namespace  = "cert-manager"
#   create_namespace = true
#
#   repository  = "https://charts.jetstack.io"
#   chart      = "cert-manager"
#   version    = "v1.13.3"
#
#   set {
#     name  = "installCRDs"
#     value = "true"
#   }
#
#   set {
#     name  = "replicaCount"
#     value = "2"
#   }
#
#   set {
#     name  = "podSecurityCertManagerController.seccompProfile.type"
#     value = "RuntimeDefault"
#   }
#
#   depends_on = [module.eks]
#
#   tags = local.common_tags
# }

# =============================================================================
# Application Helm Releases
# =============================================================================

# ──────────────────────────────────────────────
# Chaos-Sec Backend
# ──────────────────────────────────────────────
# Deploys the backend API service with environment variables
# pointing to RDS PostgreSQL and ElastiCache Redis.
resource "helm_release" "chaos_sec_backend" {
  name       = "chaos-sec-backend"
  namespace  = kubernetes_namespace.chaos_sec.metadata[0].name
  repository = "https://chaos-sec.github.io/charts"
  chart      = "chaos-sec-backend"
  version    = "1.0.0"

  set {
    name  = "image.repository"
    value = var.backend_image
  }

  set {
    name  = "image.tag"
    value = "latest"
  }

  set {
    name  = "serviceAccount.name"
    value = kubernetes_service_account.backend.metadata[0].name
  }

  # Database connection
  set {
    name  = "env.DATABASE_HOST"
    value = aws_db_instance.chaos_sec_postgres.address
  }

  set {
    name  = "env.DATABASE_PORT"
    value = tostring(aws_db_instance.chaos_sec_postgres.port)
  }

  set {
    name  = "env.DATABASE_NAME"
    value = "chaos_sec"
  }

  set {
    name  = "env.DATABASE_URL"
    value = "postgresql://chaos_sec_admin:${var.db_password}@${aws_db_instance.chaos_sec_postgres.address}:${aws_db_instance.chaos_sec_postgres.port}/chaos_sec"
  }

  # Redis connection
  set {
    name  = "env.REDIS_HOST"
    value = aws_elasticache_replication_group.chaos_sec_redis.primary_endpoint_address
  }

  set {
    name  = "env.REDIS_PORT"
    value = tostring(aws_elasticache_replication_group.chaos_sec_redis.port)
  }

  # JWT and application secrets
  set {
    name  = "env.JWT_SECRET"
    value = var.jwt_secret
  }

  set {
    name  = "env.S3_BUCKET"
    value = aws_s3_bucket.chaos_sec_data.id
  }

  set {
    name  = "env.AWS_REGION"
    value = var.aws_region
  }

  # Resource limits
  set {
    name  = "resources.requests.cpu"
    value = "250m"
  }

  set {
    name  = "resources.requests.memory"
    value = "256Mi"
  }

  set {
    name  = "resources.limits.cpu"
    value = "1000m"
  }

  set {
    name  = "resources.limits.memory"
    value = "512Mi"
  }

  # Replicas
  set {
    name  = "replicaCount"
    value = var.environment == "production" ? "3" : "1"
  }

  depends_on = [module.eks, kubernetes_namespace.chaos_sec, kubernetes_service_account.backend]

  tags = local.common_tags
}

# ──────────────────────────────────────────────
# Chaos-Sec Frontend
# ──────────────────────────────────────────────
# Deploys the frontend web application served via NGINX.
resource "helm_release" "chaos_sec_frontend" {
  name       = "chaos-sec-frontend"
  namespace  = kubernetes_namespace.chaos_sec.metadata[0].name
  repository = "https://chaos-sec.github.io/charts"
  chart      = "chaos-sec-frontend"
  version    = "1.0.0"

  set {
    name  = "image.repository"
    value = var.frontend_image
  }

  set {
    name  = "image.tag"
    value = "latest"
  }

  set {
    name  = "serviceAccount.name"
    value = kubernetes_service_account.frontend.metadata[0].name
  }

  # Backend API URL for the frontend to connect to
  set {
    name  = "env.BACKEND_API_URL"
    value = "https://${var.domain_name}/api"
  }

  set {
    name  = "env.JWT_SECRET"
    value = var.jwt_secret
  }

  # Resource limits
  set {
    name  = "resources.requests.cpu"
    value = "100m"
  }

  set {
    name  = "resources.requests.memory"
    value = "128Mi"
  }

  set {
    name  = "resources.limits.cpu"
    value = "500m"
  }

  set {
    name  = "resources.limits.memory"
    value = "256Mi"
  }

  # Replicas
  set {
    name  = "replicaCount"
    value = var.environment == "production" ? "2" : "1"
  }

  depends_on = [module.eks, kubernetes_namespace.chaos_sec, kubernetes_service_account.frontend]

  tags = local.common_tags
}

# =============================================================================
# Monitoring Helm Releases
# =============================================================================

# ──────────────────────────────────────────────
# kube-prometheus-stack (Prometheus + Grafana)
# ──────────────────────────────────────────────
# Deploys a full observability stack including:
#   - Prometheus for metrics collection and alerting
#   - Grafana for dashboards and visualization
#   - Alertmanager for alert routing and notifications
#   - Node Exporter and kube-state-metrics for cluster metrics
resource "helm_release" "prometheus_stack" {
  name             = "kube-prometheus-stack"
  namespace        = "monitoring"
  create_namespace = true

  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  version    = "55.5.0"

  # ──────────────────────────────────────────
  # Prometheus Configuration
  # ──────────────────────────────────────────
  set {
    name  = "prometheus.prometheusSpec.retention"
    value = "15d"
  }

  set {
    name  = "prometheus.prometheusSpec.retentionSize"
    value = "50GB"
  }

  set {
    name  = "prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.storageClassName"
    value = "gp3"
  }

  set {
    name  = "prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.accessModes[0]"
    value = "ReadWriteOnce"
  }

  set {
    name  = "prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage"
    value = "50Gi"
  }

  set {
    name  = "prometheus.prometheusSpec.replicas"
    value = "1"
  }

  # ──────────────────────────────────────────
  # Grafana Configuration
  # ──────────────────────────────────────────
  set {
    name  = "grafana.enabled"
    value = "true"
  }

  set {
    name  = "grafana.persistence.enabled"
    value = "true"
  }

  set {
    name  = "grafana.persistence.storageClassName"
    value = "gp3"
  }

  set {
    name  = "grafana.persistence.size"
    value = "10Gi"
  }

  set {
    name  = "grafana.adminPassword"
    value = var.jwt_secret
  }

  set {
    name  = "grafana.ingress.enabled"
    value = "true"
  }

  set {
    name  = "grafana.ingress.annotations.kubernetes\\.io/ingress.class"
    value = "nginx"
  }

  set {
    name  = "grafana.ingress.annotations.cert-manager\\.io/cluster-issuer"
    value = "letsencrypt-prod"
  }

  set {
    name  = "grafana.ingress.hosts[0]"
    value = "grafana.${var.domain_name}"
  }

  set {
    name  = "grafana.ingress.tls[0].hosts[0]"
    value = "grafana.${var.domain_name}"
  }

  set {
    name  = "grafana.ingress.tls[0].secretName"
    value = "grafana-tls"
  }

  # ──────────────────────────────────────────
  # Alertmanager Configuration
  # ──────────────────────────────────────────
  set {
    name  = "alertmanager.enabled"
    value = "true"
  }

  set {
    name  = "alertmanager.alertmanagerSpec.replicas"
    value = "1"
  }

  set {
    name  = "alertmanager.alertmanagerSpec.storage.volumeClaimTemplate.spec.storageClassName"
    value = "gp3"
  }

  set {
    name  = "alertmanager.alertmanagerSpec.storage.volumeClaimTemplate.spec.accessModes[0]"
    value = "ReadWriteOnce"
  }

  set {
    name  = "alertmanager.alertmanagerSpec.storage.volumeClaimTemplate.spec.resources.requests.storage"
    value = "5Gi"
  }

  # ──────────────────────────────────────────
  # General Settings
  # ──────────────────────────────────────────
  set {
    name  = "nodeExporter.enabled"
    value = "true"
  }

  set {
    name  = "kubeStateMetrics.enabled"
    value = "true"
  }

  depends_on = [module.eks, helm_release.ingress_nginx]

  tags = local.common_tags
}

# =============================================================================
# Variables
# =============================================================================

variable "project_name" {
  description = "Project name used for resource naming"
  type        = string
  default     = "chaos-sec"
}

variable "environment" {
  description = "Deployment environment (production, staging, development)"
  type        = string
  default     = "production"

  validation {
    condition     = contains(["production", "staging", "development"], var.environment)
    error_message = "Environment must be one of: production, staging, development."
  }
}

variable "aws_region" {
  description = "AWS region for all resources"
  type        = string
  default     = "eu-west-2"

  validation {
    condition     = can(regex("^(us|eu|ap|sa|ca|me|af)-[a-z]+-[0-9]+$", var.aws_region))
    error_message = "AWS region must be a valid format (e.g., us-east-1, eu-west-2)."
  }
}

variable "owner" {
  description = "Team or individual responsible for the resources"
  type        = string
  default     = "chaos-sec-team"
}

variable "cost_center" {
  description = "Cost center for billing and chargeback"
  type        = string
  default     = "engineering"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.0.0.0/16"

  validation {
    condition     = can(cidrnetmask(var.vpc_cidr))
    error_message = "VPC CIDR must be a valid IPv4 CIDR block."
  }
}

variable "az_count" {
  description = "Number of availability zones to use"
  type        = number
  default     = 3

  validation {
    condition     = var.az_count >= 2 && var.az_count <= 4
    error_message = "AZ count must be between 2 and 4."
  }
}

variable "kubernetes_version" {
  description = "Kubernetes version for the EKS cluster"
  type        = string
  default     = "1.28"

  validation {
    condition     = can(regex("^[0-9]+\\.[0-9]+$", var.kubernetes_version))
    error_message = "Kubernetes version must be in format X.Y (e.g., 1.28)."
  }
}

variable "allowed_public_access_cidrs" {
  description = "CIDR blocks allowed to access the Kubernetes API publicly"
  type        = list(string)
  default     = ["0.0.0.0/0"]

  # For production, restrict to office/VPN CIDRs:
  # default   = ["203.0.113.0/24"]
}

variable "admin_role_name" {
  description = "IAM role name for Kubernetes cluster admin access"
  type        = string
  default     = "ChaosSecEksAdminRole"
}

variable "node_group_ami_type" {
  description = "AMI type for EKS node groups (AL2_x86_64, AL2_ARM_64)"
  type        = string
  default     = "AL2_ARM_64"

  validation {
    condition     = contains(["AL2_x86_64", "AL2_ARM_64"], var.node_group_ami_type)
    error_message = "AMI type must be AL2_x86_64 or AL2_ARM_64."
  }
}

# ──────────────────────────────────────────────
# Application Node Group Configuration
# ──────────────────────────────────────────────

variable "app_node_instance_types" {
  description = "EC2 instance types for the application node group"
  type        = list(string)
  default     = ["m6g.large"]
}

variable "app_node_min_size" {
  description = "Minimum number of nodes in the application node group"
  type        = number
  default     = 2
}

variable "app_node_max_size" {
  description = "Maximum number of nodes in the application node group"
  type        = number
  default     = 10
}

variable "app_node_desired_size" {
  description = "Desired number of nodes in the application node group"
  type        = number
  default     = 3
}

variable "app_node_disk_size" {
  description = "EBS disk size (GiB) for application nodes"
  type        = number
  default     = 50
}

# ──────────────────────────────────────────────
# Data Node Group Configuration
# ──────────────────────────────────────────────

variable "data_node_instance_types" {
  description = "EC2 instance types for the data node group"
  type        = list(string)
  default     = ["r6g.xlarge"]
}

variable "data_node_min_size" {
  description = "Minimum number of nodes in the data node group"
  type        = number
  default     = 1
}

variable "data_node_max_size" {
  description = "Maximum number of nodes in the data node group"
  type        = number
  default     = 3
}

variable "data_node_desired_size" {
  description = "Desired number of nodes in the data node group"
  type        = number
  default     = 1
}

variable "data_node_disk_size" {
  description = "EBS disk size (GiB) for data nodes"
  type        = number
  default     = 100
}

# ──────────────────────────────────────────────
# Chaos Experiment Node Group Configuration
# ──────────────────────────────────────────────

variable "chaos_node_instance_types" {
  description = "EC2 instance types for the chaos experiment node group"
  type        = list(string)
  default     = ["m6g.medium"]
}

variable "chaos_node_min_size" {
  description = "Minimum number of nodes in the chaos node group"
  type        = number
  default     = 1
}

variable "chaos_node_max_size" {
  description = "Maximum number of nodes in the chaos node group"
  type        = number
  default     = 5
}

variable "chaos_node_desired_size" {
  description = "Desired number of nodes in the chaos node group"
  type        = number
  default     = 1
}

variable "chaos_node_disk_size" {
  description = "EBS disk size (GiB) for chaos experiment nodes"
  type        = number
  default     = 30
}

# ──────────────────────────────────────────────
# Database Configuration
# ──────────────────────────────────────────────

variable "db_password" {
  description = "Password for the RDS PostgreSQL admin user"
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.db_password) >= 8
    error_message = "Database password must be at least 8 characters."
  }
}

variable "db_instance_class" {
  description = "RDS instance class for PostgreSQL"
  type        = string
  default     = "db.t3.medium"
}

# ──────────────────────────────────────────────
# Redis Configuration
# ──────────────────────────────────────────────

variable "redis_password" {
  description = "Auth token for ElastiCache Redis"
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.redis_password) >= 16
    error_message = "Redis auth token must be at least 16 characters."
  }
}

variable "redis_node_type" {
  description = "ElastiCache node type for Redis"
  type        = string
  default     = "cache.t3.medium"
}

# ──────────────────────────────────────────────
# Application Configuration
# ──────────────────────────────────────────────

variable "jwt_secret" {
  description = "JWT signing secret for Chaos-Sec authentication"
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.jwt_secret) >= 16
    error_message = "JWT secret must be at least 16 characters."
  }
}

variable "backend_image" {
  description = "Container image for the Chaos-Sec backend"
  type        = string
  default     = "chaos-sec-backend:latest"
}

variable "frontend_image" {
  description = "Container image for the Chaos-Sec frontend"
  type        = string
  default     = "chaos-sec-frontend:latest"
}

variable "domain_name" {
  description = "Domain name for ingress and SSL certificate configuration"
  type        = string
  default     = "chaos-sec.example.com"
}

# =============================================================================
# Outputs
# =============================================================================

output "cluster_name" {
  description = "EKS cluster name"
  value       = module.eks.cluster_name
}

output "cluster_endpoint" {
  description = "EKS cluster API endpoint"
  value       = module.eks.cluster_endpoint
}

output "cluster_arn" {
  description = "EKS cluster ARN"
  value       = module.eks.cluster_arn
}

output "cluster_version" {
  description = "Kubernetes version of the EKS cluster"
  value       = module.eks.cluster_version
}

output "cluster_security_group_id" {
  description = "Security group ID attached to the EKS cluster"
  value       = module.eks.cluster_security_group_id
}

output "oidc_provider_arn" {
  description = "OIDC provider ARN for IRSA"
  value       = module.eks.oidc_provider_arn
}

output "oidc_provider_url" {
  description = "OIDC provider URL for IRSA"
  value       = module.eks.oidc_provider_url
}

output "vpc_id" {
  description = "VPC ID"
  value       = module.vpc.vpc_id
}

output "vpc_cidr" {
  description = "VPC CIDR block"
  value       = module.vpc.vpc_id
}

output "public_subnet_ids" {
  description = "Public subnet IDs"
  value       = module.vpc.public_subnet_ids
}

output "private_subnet_ids" {
  description = "Private subnet IDs"
  value       = module.vpc.private_subnet_ids
}

output "database_subnet_ids" {
  description = "Database subnet IDs"
  value       = module.vpc.database_subnet_ids
}

output "app_node_group_arn" {
  description = "ARN of the application node group"
  value       = module.eks.eks_managed_node_groups["app"].node_group_arn
}

output "data_node_group_arn" {
  description = "ARN of the data node group"
  value       = module.eks.eks_managed_node_groups["data"].node_group_arn
}

output "chaos_node_group_arn" {
  description = "ARN of the chaos experiment node group"
  value       = module.eks.eks_managed_node_groups["chaos"].node_group_arn
}

output "backend_irsa_role_arn" {
  description = "IAM role ARN for the backend service account (IRSA)"
  value       = module.backend_irsa.iam_role_arn
}

output "lb_controller_irsa_role_arn" {
  description = "IAM role ARN for the AWS Load Balancer Controller (IRSA)"
  value       = module.load_balancer_controller_irsa.iam_role_arn
}

output "s3_bucket_name" {
  description = "S3 bucket name for Chaos-Sec data storage"
  value       = aws_s3_bucket.chaos_sec_data.id
}

output "s3_bucket_arn" {
  description = "S3 bucket ARN for Chaos-Sec data storage"
  value       = aws_s3_bucket.chaos_sec_data.arn
}

output "kms_key_arn" {
  description = "KMS key ARN used for EKS cluster and EBS encryption"
  value       = module.eks.kms_key_arn
}

# ──────────────────────────────────────────────
# Kubeconfig Command
# ──────────────────────────────────────────────
output "kubeconfig_command" {
  description = "Command to update kubeconfig for EKS cluster access"
  value       = "aws eks update-kubeconfig --region ${var.aws_region} --name ${module.eks.cluster_name}"
}

output "kubectl_apply_command" {
  description = "Command to apply the Chaos-Sec Kubernetes manifests"
  value       = "kubectl apply -k deploy/kubernetes/"
}

# ──────────────────────────────────────────────
# Database & Cache Endpoints
# ──────────────────────────────────────────────
output "rds_endpoint" {
  description = "RDS PostgreSQL endpoint"
  value       = aws_db_instance.chaos_sec_postgres.endpoint
}

output "rds_port" {
  description = "RDS PostgreSQL port"
  value       = aws_db_instance.chaos_sec_postgres.port
}

output "elasticache_endpoint" {
  description = "ElastiCache Redis primary endpoint"
  value       = aws_elasticache_replication_group.chaos_sec_redis.primary_endpoint_address
}

output "elasticache_port" {
  description = "ElastiCache Redis port"
  value       = aws_elasticache_replication_group.chaos_sec_redis.port
}

output "db_credentials_secret_arn" {
  description = "ARN of the Secrets Manager secret containing DB credentials"
  value       = aws_secretsmanager_secret.db_credentials.arn
}

output "jwt_secret_arn" {
  description = "ARN of the Secrets Manager secret containing the JWT secret"
  value       = aws_secretsmanager_secret.jwt_secret.arn
}

output "redis_credentials_secret_arn" {
  description = "ARN of the Secrets Manager secret containing Redis credentials"
  value       = aws_secretsmanager_secret.redis_credentials.arn
}
