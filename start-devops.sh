#!/bin/bash

# Starta kind-containrar om de inte kör
docker start devops-cluster-control-plane devops-cluster-worker devops-cluster-worker2 2>/dev/null

# Hämta aktuell IP för control-plane
CONTROL_PLANE_IP=$(docker inspect devops-cluster-control-plane --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')

echo "Control-plane IP: $CONTROL_PLANE_IP"

# Uppdatera kubeconfig med rätt IP
sed "s/127.0.0.1/$CONTROL_PLANE_IP/g" ~/.kube/config > ~/kubeconfig-jenkins

# Starta Jenkins
docker start jenkins
docker network connect kind jenkins 2>/dev/null || true

# Ladda in uppdaterad kubeconfig
sleep 5
docker cp ~/kubeconfig-jenkins jenkins:/var/jenkins_home/.kube/config
docker exec -u root jenkins chmod 666 /var/run/docker.sock

echo "Klart. Jenkins tillgänglig på http://localhost:8080"
