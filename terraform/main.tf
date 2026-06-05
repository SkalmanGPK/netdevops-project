terraform {
  required_version = ">= 1.0.0"
  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "0.4.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "2.26.0"
    }
  }
}

# ==========================================
# 1. PROVIDER INITIALIZATION
# ==========================================

provider "kind" {}

# ==========================================
# 2. INFRASTRUCTURE & NETWORK PROVISIONING
# ==========================================

resource "kind_cluster" "devops_cluster" {
  name           = "devops-cluster"
  wait_for_ready = true

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    networking {
      api_server_address = "127.0.0.1"
      api_server_port    = 6443
      pod_subnet         = "10.244.0.0/16"
      service_subnet     = "10.96.0.0/12"
    }

    node {
      role = "control-plane"

      extra_port_mappings {
        container_port = 80
        host_port      = 80
        protocol       = "TCP"
      }

      extra_port_mappings {
        container_port = 443
        host_port      = 443
        protocol       = "TCP"
      }
    }

    node {
      role = "worker"
    }

    node {
      role = "worker"
    }
  }
}

# Dynamic authentication configuration leveraging the newly generated cluster credentials
provider "kubernetes" {
  host                   = kind_cluster.devops_cluster.endpoint
  client_certificate     = kind_cluster.devops_cluster.client_certificate
  client_key             = kind_cluster.devops_cluster.client_key
  cluster_ca_certificate = kind_cluster.devops_cluster.cluster_ca_certificate
}

# ==========================================
# 3. NATIVE KUBERNETES CORE RESOURCES (RBAC)
# ==========================================

resource "kubernetes_service_account" "mesh_pinger" {
  metadata {
    name      = "mesh-pinger-sa"
    namespace = "default"
  }
}

resource "kubernetes_cluster_role" "mesh_pinger" {
  metadata {
    name = "mesh-pinger-cluster-role"
  }

  rule {
    api_groups = [""]
    resources  = ["pods"]
    verbs      = ["list"]
  }
}

resource "kubernetes_cluster_role_binding" "mesh_pinger" {
  metadata {
    name = "mesh-pinger-crb"
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.mesh_pinger.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.mesh_pinger.metadata[0].name
    namespace = "default"
  }
}

# ==========================================
# 4. HARDENED APP & SIDECAR DEPLOYMENT
# ==========================================

resource "kubernetes_deployment" "mesh_pinger" {
  wait_for_rollout = true
  metadata {
    name = "mesh-pinger-deployment"
    labels = {
      app = "mesh-pinger"
    }
  }

  spec {
    replicas = 3

    selector {
      match_labels = {
        app = "mesh-pinger"
      }
    }

    template {
      metadata {
        labels = {
          app = "mesh-pinger"
        }
      }

      spec {
        service_account_name = kubernetes_service_account.mesh_pinger.metadata[0].name

        # Stricter Linux security context at the pod layer
        security_context {
          run_as_non_root = true
          run_as_user     = 10001
        }

        # Container 1: The Main Application Target
        container {
          name  = "main-app"
          image = "hashicorp/http-echo:latest"
          args  = ["-text=OK", "-listen=:8081"]

          port {
            container_port = 8081
            name           = "app-http"
          }

          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem   = true
            capabilities {
              drop = ["ALL"]
            }
          }

          resources {
            requests = {
              memory = "32Mi"
              cpu    = "50m"
            }
            limits = {
              memory = "64Mi"
              cpu    = "100m"
            }
          }
        }

        # Container 2: Your Go Network Telemetry Agent (Pinger Sidecar)
        container {
          name              = "pinger"
          image             = "mesh-pinger:v3"
          image_pull_policy = "Never"

          port {
            container_port = 8090
            name           = "pinger-http"
          }

          env {
            name = "NODE_NAME"
            value_from {
              field_ref {
                field_path = "spec.nodeName"
              }
            }
          }

          env {
            name = "POD_IP"
            value_from {
              field_ref {
                field_path = "status.podIP"
              }
            }
          }

          env {
            name  = "TARGET_LABEL"
            value = "mesh-pinger"
          }

          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem   = true
            capabilities {
              drop = ["ALL"]
            }
          }

          resources {
            requests = {
              memory = "64Mi"
              cpu    = "100m"
            }
            limits = {
              memory = "128Mi"
              cpu    = "200m"
            }
          }

          startup_probe {
            http_get {
              path = "/health"
              port = "8090"
            }
            initial_delay_seconds = 10
            period_seconds        = 5
            failure_threshold     = 10
          }

          readiness_probe {
            http_get {
              path = "/health"
              port = "8090"
            }
            initial_delay_seconds = 2
            period_seconds        = 5
            success_threshold     = 1
            failure_threshold     = 2
          }

          liveness_probe {
            http_get {
              path = "/health"
              port = "8090"
            }
            initial_delay_seconds = 5
            period_seconds        = 10
            timeout_seconds       = 2
            failure_threshold     = 3
          }
        }
      }
    }
  }
}

# ==========================================
# 5. ARCHITECTURAL OUTPUTS
# ==========================================

output "kubernetes_api_endpoint" {
  value       = kind_cluster.devops_cluster.endpoint
  description = "The URL of the Kubernetes API server"
}

output "deployment_status" {
  value       = "NetDevOps cluster and native resources planned and deployed in a single sequence execution."
  description = "Execution confirmation status"
}