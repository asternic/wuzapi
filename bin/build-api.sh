#/bin/bash

# Load environment variables
set -a; . ./.env; set +a
export VERSION=v202602120600

# Build and push the image to ECR
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 058264349674.dkr.ecr.us-east-1.amazonaws.com
docker build -t zapperapi/wuzapi .
docker tag zapperapi/wuzapi:latest $ECR_IMAGE_NAME:latest
docker tag zapperapi/wuzapi:latest $ECR_IMAGE_NAME:$VERSION
docker push $ECR_IMAGE_NAME:$VERSION
docker push $ECR_IMAGE_NAME:latest
