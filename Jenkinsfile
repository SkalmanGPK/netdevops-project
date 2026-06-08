pipeline {
    agent any

    triggers {
        pollSCM('H/5 * * * *') // polls every fifth second
    }
    environment {
        IMAGE_NAME = "mesh-pinger"
        IMAGE_TAG  = GIT_COMMIT[0..6]
        CLUSTER    = "devops-cluster"
    }

    stages {
        stage('Build') {
            steps {
                sh 'docker build -t ${IMAGE_NAME}:${IMAGE_TAG} ./network-pinger'
            }
        }

        stage('Load into kind') {
            steps {
                sh 'kind load docker-image ${IMAGE_NAME}:${IMAGE_TAG} --name ${CLUSTER}'
            }
        }

        stage('Deploy') {
            steps {
                sh 'kubectl rollout restart deployment mesh-pinger-deployment'
                sh 'kubectl rollout status deployment mesh-pinger-deployment --timeout=60s'
            }
        }
    }

    post {
        success {
            echo 'Pipeline klar - mesh-pinger är uppdaterad'
        }
        failure {
            echo 'Pipeline misslyckades'
        }
    }
}