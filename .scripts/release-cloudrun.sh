#!/bin/bash
set -euo pipefail

# =============================================================================
# Build and Deploy to Google Cloud Run
# =============================================================================
# Usage: ./.scripts/release-cloudrun.sh
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

# =============================================================================
# CONFIGURATION - แก้ไขค่าตรงนี้
# =============================================================================

# GCP Configuration
export GCP_PROJECT_ID="starwolf-481609"
export GCP_REGION="asia-southeast3"
export GAR_REPOSITORY="cloud-run-source-deploy"

# Service Configuration
export SERVICE_NAME="hm-core-platform-mgmt"

# Cloud Run Configuration
export CLOUD_RUN_MEMORY="256Mi"
export CLOUD_RUN_CPU="1"
export CLOUD_RUN_MIN_INSTANCES="0"
export CLOUD_RUN_MAX_INSTANCES="1"
export CLOUD_RUN_CONCURRENCY="80"
export CLOUD_RUN_TIMEOUT="300"


# =============================================================================
# DO NOT EDIT BELOW THIS LINE
# =============================================================================

# Get version and git commit
VERSION=$(cat VERSION | tr -d '\n')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
export IMAGE_TAG="${VERSION}-${GIT_COMMIT}"

# Image URI
IMAGE_URI="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT_ID}/${GAR_REPOSITORY}/${SERVICE_NAME}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
print_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Validate
if [[ "$GCP_PROJECT_ID" == "your-gcp-project-id" ]]; then
    print_error "กรุณาแก้ไข GCP_PROJECT_ID ในไฟล์ release-cloudrun.sh"
    exit 1
fi

print_info "=== Release to Cloud Run ==="
print_info "Project: ${GCP_PROJECT_ID}"
print_info "Region: ${GCP_REGION}"
print_info "Service: ${SERVICE_NAME}"
print_info "Image: ${IMAGE_URI}:${IMAGE_TAG}"
echo ""

# Step 1: Configure Docker
print_info "Configuring Docker for Artifact Registry..."
# gcloud auth configure-docker "${GCP_REGION}-docker.pkg.dev" --quiet
gcloud auth print-access-token | docker login -u oauth2accesstoken --password-stdin https://${GCP_REGION}-docker.pkg.dev

# Step 2: Build image (public Go modules — no GIT_USERNAME/PASSWORD build-args)
print_info "Building Docker image..."
docker buildx build \
    --platform linux/amd64 \
    --load \
    --build-arg GIT_COMMIT="${GIT_COMMIT}" \
    -t "${IMAGE_URI}:${IMAGE_TAG}" \
    .

# Step 3: Push image
print_info "Pushing Docker image..."
docker push "${IMAGE_URI}:${IMAGE_TAG}"

# Step 4: Deploy to Cloud Run
print_info "Deploying to Cloud Run..."
gcloud run deploy "${SERVICE_NAME}" \
    --project="${GCP_PROJECT_ID}" \
    --region="${GCP_REGION}" \
    --image="${IMAGE_URI}:${IMAGE_TAG}" \
    --platform=managed \
    --memory="${CLOUD_RUN_MEMORY}" \
    --cpu="${CLOUD_RUN_CPU}" \
    --min-instances="${CLOUD_RUN_MIN_INSTANCES}" \
    --max-instances="${CLOUD_RUN_MAX_INSTANCES}" \
    --concurrency="${CLOUD_RUN_CONCURRENCY}" \
    --timeout="${CLOUD_RUN_TIMEOUT}" \
    --port=8080 \
    # --allow-unauthenticated

# Step 5: Get service URL
print_info "Getting service URL..."
SERVICE_URL=$(gcloud run services describe "${SERVICE_NAME}" \
    --project="${GCP_PROJECT_ID}" \
    --region="${GCP_REGION}" \
    --format="value(status.url)")

echo ""
print_info "=== Release Complete ==="
print_info "Service URL: ${SERVICE_URL}"
