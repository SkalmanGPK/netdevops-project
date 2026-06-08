#!/bin/bash


docker start devops-cluster-control-plane devops-cluster-worker devops-cluster-worker2 2>/dev/null

#Get IP address from control plane
CONTROL_PLANE_IP=$(docker inspect devops-cluster-control-plane --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')

echo "Control-plane IP: $CONTROL_PLANE_IP"

# Uppdate kubeconfig with control plane IP and copy to Jenkins
sed "s/127.0.0.1/$CONTROL_PLANE_IP/g" ~/.kube/config > ~/kubeconfig-jenkins

# Start Jenkins
docker start jenkins
docker network connect kind jenkins 2>/dev/null || true

#Load in uppdated kubeconfig to Jenkins and give access to docker socket
sleep 5
docker cp ~/kubeconfig-jenkins jenkins:/var/jenkins_home/.kube/config
docker exec -u root jenkins chmod 666 /var/run/docker.sock

echo "Klart. Jenkins tillgänglig på http://localhost:8080"