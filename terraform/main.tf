terraform {
  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "0.4.0"
    }
    null = {
      source  = "hashicorp/null"
      version = "3.2.2"
    }
  }
}

provider "kind" {}
provider "null" {}

# Denna resurs hanterar klustrets livscykel via Kind CLI
resource "null_resource" "kind_cluster" {
  
  # Körs vid 'terraform apply'
  provisioner "local-exec" {
    command = "kind create cluster --name devops-cluster --config ${path.module}/kind-config.yaml"
  }

  # Körs vid 'terraform destroy'
  # Notera: 'when = destroy' kräver ett block med ett 'command' argument
  provisioner "local-exec" {
    when    = destroy
    command = "kind delete cluster --name devops-cluster"
  }
}

# Output för att bekräfta namnet i terminalen efter körning
output "cluster_name" {
  value = "devops-cluster"
}
