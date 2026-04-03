# Kubernetes Mesh-Pinger (NetDevOps Project)

Detta projekt är en **Service Mesh-agent** byggd för att demonstrera hur man mäter nätverksprestanda (latens) mellan noder i ett Kubernetes-kluster med hjälp av Go, Terraform och Kubernetes API.

## 🚀 Övergripande Arkitektur
Projektet automatiserar hela kedjan från infrastruktur till applikation:
1. **Infrastruktur:** Terraform sätter upp ett lokalt `kind`-kluster med flera noder.
2. **Service Discovery:** Go-applikationen pratar med Kubernetes API för att hitta sina grannar.
3. **Data Plane:** Varje instans av appen mäter latens mot sina grannar via HTTP.



---

## 📁 Projektstruktur

*   **`terraform/`**: Innehåller IaC-kod för att provisionera ett Kind-kluster med 1 control-plane och 2 workers.
*   **`network-pinger/`**: Go-källkod för pinger-agenten och en Multi-stage Dockerfile för minimal image-storlek.
*   **`k8s-manifests/`**: Kubernetes-resurser inklusive en `DaemonSet` (för att köra appen på alla noder) och `RBAC`-rättigheter.

---

## 🛠 Installation & Setup

### 1. Skapa Klustret
Gå till terraform-mappen och kör:
```bash
terraform init
terraform apply
2. Förbered Go-projektet
Om go.mod saknas i network-pinger/, generera den med:

Bash
cd network-pinger
go mod init network-pinger
go mod tidy
3. Bygg och Ladda Image
Bygg din container och gör den tillgänglig för Kind:

Bash
docker build -t mesh-pinger:v3 .
kind load docker-image mesh-pinger:v3 --name devops-cluster
4. Deploy till Kubernetes
Applicera rättigheter och starta pinger-agenterna:

Bash
kubectl apply -f k8s-manifests/rbac.yaml
kubectl apply -f k8s-manifests/pinger-daemonset.yaml
📊 Användning
För att se nätverkslatensen mellan dina noder i realtid, titta på loggarna för en av poddarna:

Bash
kubectl logs -l app=mesh-pinger -f
Exempel på utdata:

Plaintext
--- Mesh Check: 2 pods found ---
LATENCY to devops-cluster-worker2 (10.244.1.3): 0.842ms (Status: 200 OK)
🛡 Säkerhet (RBAC)
Applikationen använder en dedikerad ServiceAccount med en ClusterRole som begränsar dess åtkomst till att endast kunna "lista" och "watch" poddar. Detta följer principen om Least Privilege.


### Ett tips för ditt GitHub-repo:
För att få med `go.mod` och `go.sum` på GitHub (vilket rekommenderas så att andra kan bygga koden direkt), kör följande i din terminal:

1. Gå till `network-pinger/`
2. Kör `go mod tidy` (om du inte redan gjort det)
3. Kör `git add go.mod go.sum`
4. Kör `git commit -m "Add Go module files"`
5. Kör `git push`

Då blir ditt repo helt komplett! Vill du att vi lägger till något specifikt om dina Ansible-planer i README:n också?
