pipeline {
    agent any

    options {
        timestamps()
        disableConcurrentBuilds()
        skipDefaultCheckout(true)
    }

    environment {
        IMAGE_NAME = 'rizkysiregar/feedzmore-media-worker'
        IMAGE_PLATFORM = 'linux/amd64'
        GITOPS_REPOSITORY = 'https://github.com/rizkysiregar/homelabs-gitops.git'
        GITOPS_BRANCH = 'main'
        GITOPS_MANIFEST = 'apps/feedzmore/kustomization.yaml'
        GO_IMAGE = 'golang:1.26-alpine'
        YQ_IMAGE = 'mikefarah/yq:4.45.4'
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
                script {
                    env.IMAGE_TAG = sh(
                        script: 'git rev-parse --short=12 HEAD',
                        returnStdout: true
                    ).trim()
                }
                echo "Preparing image ${IMAGE_NAME}:${IMAGE_TAG}"
            }
        }

        stage('Test') {
            steps {
                sh '''
                    docker run --rm \\
                      --user "$(id -u):$(id -g)" \\
                      --volumes-from "${JENKINS_CONTAINER_ID:-$HOSTNAME}" \\
                      --workdir "$WORKSPACE" \\
                      --env GOCACHE=/tmp/.gocache \\
                      --env GOMODCACHE=/tmp/.gomod \\
                      "$GO_IMAGE" \\
                      sh -c 'go vet ./... && go test ./...'
                '''
            }
        }

        stage('Build and Push Image') {
            steps {
                withCredentials([
                    usernamePassword(
                        credentialsId: 'dockerhub',
                        usernameVariable: 'DOCKER_USERNAME',
                        passwordVariable: 'DOCKER_PASSWORD'
                    )
                ]) {
                    sh '''
                        set +x
                        printf '%s' "$DOCKER_PASSWORD" | docker login docker.io \\
                          --username "$DOCKER_USERNAME" \\
                          --password-stdin
                        set -x

                        docker build \\
                          --platform "$IMAGE_PLATFORM" \\
                          --tag "$IMAGE_NAME:$IMAGE_TAG" \\
                          .
                        docker push "$IMAGE_NAME:$IMAGE_TAG"
                        docker logout docker.io || true
                    '''
                }
            }
        }

        stage('Update GitOps Image Tag') {
            steps {
                dir('gitops') {
                    deleteDir()
                    git branch: "${GITOPS_BRANCH}",
                        credentialsId: 'github-all-repo-pat',
                        url: "${GITOPS_REPOSITORY}"

                    withCredentials([
                        gitUsernamePassword(
                            credentialsId: 'github-all-repo-pat',
                            gitToolName: 'Default'
                        )
                    ]) {
                        sh '''
                            docker run --rm \\
                              --user "$(id -u):$(id -g)" \\
                              --volumes-from "${JENKINS_CONTAINER_ID:-$HOSTNAME}" \\
                              --workdir "$PWD" \\
                              --env IMAGE_NAME \\
                              --env IMAGE_TAG \\
                              "$YQ_IMAGE" \\
                              -i \\
                              '(.images[] | select(.name == strenv(IMAGE_NAME))).newTag = strenv(IMAGE_TAG)' \\
                              "$GITOPS_MANIFEST"

                            git diff --check
                            git config user.name 'Jenkins'
                            git config user.email 'jenkins@feedzmore.ci'
                            git add "$GITOPS_MANIFEST"

                            if git diff --cached --quiet; then
                                echo "GitOps already points to $IMAGE_TAG; nothing to commit."
                                exit 0
                            fi

                            git commit -m "deploy(feedzmore-media-worker): $IMAGE_TAG"
                            git push origin "HEAD:$GITOPS_BRANCH"
                        '''
                    }
                }
            }
        }
    }
}
