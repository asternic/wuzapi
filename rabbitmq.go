package main

import (
	"os"
	"sync"
	"encoding/json"
	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

var (
	rabbitConn    *amqp091.Connection
	rabbitChannel *amqp091.Channel
	rabbitEnabled bool
	rabbitOnce    sync.Once
	rabbitQueue   string
)

// Call this in main() or initialization
func InitRabbitMQ() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	rabbitQueue = os.Getenv("RABBITMQ_QUEUE")
	if rabbitQueue == "" {
		rabbitQueue = "whatsapp_events" // default queue
	}
	if rabbitURL == "" {
		rabbitEnabled = false
		log.Info().Msg("RABBITMQ_URL is not set. RabbitMQ publishing disabled.")
		return
	}
	var err error
	rabbitConn, err = amqp091.Dial(rabbitURL)
	if err != nil {
		rabbitEnabled = false
		log.Error().Err(err).Msg("Could not connect to RabbitMQ")
		return
	}
	rabbitChannel, err = rabbitConn.Channel()
	if err != nil {
		rabbitEnabled = false
		log.Error().Err(err).Msg("Could not open RabbitMQ channel")
		return
	}
	rabbitEnabled = true
	log.Info().
		Str("queue", rabbitQueue).
		Msg("RabbitMQ connection established.")
}

// Optionally, allow overriding the queue per message
func PublishToRabbit(data []byte, queueOverride ...string) error {
	if !rabbitEnabled {
		return nil
	}
	queueName := rabbitQueue
	if len(queueOverride) > 0 && queueOverride[0] != "" {
		queueName = queueOverride[0]
	}
	// Declare queue (idempotent)
	_, err := rabbitChannel.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Error().Err(err).Str("queue", queueName).Msg("Could not declare RabbitMQ queue")
		return err
	}
	err = rabbitChannel.Publish(
		"",        // exchange (default)
		queueName, // routing key = queue
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType:  "application/json",
			Body:         data,
			DeliveryMode: amqp091.Persistent,
		},
	)
	if err != nil {
		log.Error().Err(err).Str("queue", queueName).Msg("Could not publish to RabbitMQ")
	} else {
		log.Debug().Str("queue", queueName).Msg("Published message to RabbitMQ")
	}
	return err
}

func sendToGlobalRabbit(jsonData []byte, token string, userID string, queueName ...string) {
	if !rabbitEnabled {
		log.Debug().Msg("RabbitMQ publishing is disabled, not sending message")
		return
	}

	// Extract instance information
	instance_name := ""
	userinfo, found := userinfocache.Get(token)
	if found {
		instance_name = userinfo.(Values).Get("Name")
	}

	// Parse the original JSON into a map
	var originalData map[string]interface{}
	err := json.Unmarshal(jsonData, &originalData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal original JSON data for RabbitMQ")
		return
	}

	// Add the new fields directly to the original data
	originalData["userID"] = userID
	originalData["instanceName"] = instance_name

	// Marshal back to JSON
	enhancedJSON, err := json.Marshal(originalData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal enhanced data for RabbitMQ")
		return
	}

	err = PublishToRabbit(enhancedJSON, queueName...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to publish to RabbitMQ")
	}
}
