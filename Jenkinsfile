pipeline {
    agent any

    options {
        timestamps()
        disableConcurrentBuilds()
        skipDefaultCheckout(true)
    }

    environment {
        IMAGE_REGISTRY = 'ghcr.io'
        IMAGE_NAME = 'ghcr.io/rizkysiregar/narrativ-media-worker'
        IMAGE_PLATFORM = 'linux/amd64'
        GITOPS_REPOSITORY = 'https://github.com/rizkysiregar/homelabs-gitops.git'
        GITOPS_BRANCH = 'main'
        GITOPS_MANIFEST = 'apps/narrativ-dev/kustomization.yaml'
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
                        credentialsId: 'github-all-repo-pat',
                        usernameVariable: 'REGISTRY_USERNAME',
                        passwordVariable: 'REGISTRY_PASSWORD'
                    )
                ]) {
                    sh '''
                        set +x
                        printf '%s' "$REGISTRY_PASSWORD" | docker login "$IMAGE_REGISTRY" \\
                          --username "$REGISTRY_USERNAME" \\
                          --password-stdin
                        set -x

                        docker build \\
                          --platform "$IMAGE_PLATFORM" \\
                          --tag "$IMAGE_NAME:$IMAGE_TAG" \\
                          .
                        docker push "$IMAGE_NAME:$IMAGE_TAG"
                        docker logout "$IMAGE_REGISTRY" || true
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
                              '(.images[] | select(.name == strenv(IMAGE_NAME))).newTag = strenv(IMAGE_TAG) | (.images[] | select(.name == strenv(IMAGE_NAME)) | .newTag) style="double"' \\
                              "$GITOPS_MANIFEST"

                            git diff --check
                            git config user.name 'Jenkins'
                            git config user.email 'jenkins@narrativ.ci'
                            git add "$GITOPS_MANIFEST"

                            if git diff --cached --quiet; then
                                echo "GitOps already points to $IMAGE_TAG; nothing to commit."
                                exit 0
                            fi

                            git commit -m "deploy(narrativ-dev-media-worker): $IMAGE_TAG"
                            git push origin "HEAD:$GITOPS_BRANCH"
                        '''
                    }
                }
            }
        }
    }

    post {
        always {
            // Keep only this build's image on the agent. The agent builds with the legacy builder, where an image's
            // layers are the cache that keeps the next build fast, so the latest one stays and every older tag of the
            // same image goes. Skipped when this build made no image, so a failed build never deletes the cache.
            sh '''
                if docker image inspect "$IMAGE_NAME:${IMAGE_TAG:-none}" > /dev/null 2>&1; then
                    docker images "$IMAGE_NAME" --format '{{.Tag}}' \\
                      | grep -vx "$IMAGE_TAG" \\
                      | xargs -r -I{} docker image rm "$IMAGE_NAME:{}" || true
                fi
                docker builder prune --force --filter until=72h || true
            '''
        }
    }
}
