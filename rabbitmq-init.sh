#!/bin/sh
# RabbitMQ initialization script to set up automatic message expiration policies
# This script sets TTL (Time To Live) and max-length policies for queues
# Uses RabbitMQ Management HTTP API for cross-container access

set -e

RABBITMQ_HOST=${RABBITMQ_HOST:-rabbitmq}
RABBITMQ_USER=${RABBITMQ_DEFAULT_USER:-wuzapi}
RABBITMQ_PASS=${RABBITMQ_DEFAULT_PASS:-wuzapi}
RABBITMQ_VHOST=${RABBITMQ_DEFAULT_VHOST:-/}
RABBITMQ_PORT=15672

echo "Waiting for RabbitMQ Management API to be ready..."
MAX_RETRIES=30
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
  if curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" "http://${RABBITMQ_HOST}:${RABBITMQ_PORT}/api/overview" > /dev/null 2>&1; then
    echo "RabbitMQ Management API is ready!"
    break
  fi
  echo "Waiting for RabbitMQ Management API... (attempt $((RETRY_COUNT + 1))/$MAX_RETRIES)"
  sleep 2
  RETRY_COUNT=$((RETRY_COUNT + 1))
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
  echo "ERROR: RabbitMQ Management API did not become ready in time"
  exit 1
fi

echo "Setting up RabbitMQ policies..."

# URL encode the vhost (replace / with %2F)
VHOST_ENCODED=$(echo "$RABBITMQ_VHOST" | sed 's|/|%2F|g')

# Set message TTL policy: Messages older than 24 hours will be automatically deleted
# 86400000 milliseconds = 24 hours
echo "Setting message TTL policy (24 hours)..."
curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  -X PUT \
  -H "Content-Type: application/json" \
  -d '{"pattern":".*","definition":{"message-ttl":86400000},"apply-to":"queues","priority":1}' \
  "http://${RABBITMQ_HOST}:${RABBITMQ_PORT}/api/policies/${VHOST_ENCODED}/message-ttl" > /dev/null || echo "Policy may already exist"

# Set max queue length policy: Queues will be limited to 50000 messages
# Older messages will be dropped when limit is reached
echo "Setting max queue length policy (50000 messages)..."
curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  -X PUT \
  -H "Content-Type: application/json" \
  -d '{"pattern":".*","definition":{"max-length":50000},"apply-to":"queues","priority":1}' \
  "http://${RABBITMQ_HOST}:${RABBITMQ_PORT}/api/policies/${VHOST_ENCODED}/max-length" > /dev/null || echo "Policy may already exist"

echo ""
echo "=========================================="
echo "RabbitMQ policies configured successfully!"
echo "=========================================="
echo "  ✓ Message TTL: 24 hours (messages older than 24h will be auto-deleted)"
echo "  ✓ Max queue length: 50000 messages (oldest messages dropped when limit reached)"
echo ""
echo "To verify policies, run:"
echo "  docker exec wuzapi-rabbitmq-1 rabbitmqctl list_policies"
echo ""
