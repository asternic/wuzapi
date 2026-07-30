package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

var (
	rabbitConn    *amqp091.Connection
	rabbitChannel *amqp091.Channel
	rabbitEnabled bool
	rabbitOnce    sync.Once
	rabbitQueue   string
	lastRabbitErr error // último erro de conexão, para log quando evento não é publicado
)

const (
	maxRetries    = 10
	retryInterval = 3 * time.Second
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

	// Attempt to connect with retry
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Info().
			Int("attempt", attempt).
			Int("max_retries", maxRetries).
			Msg("Attempting to connect to RabbitMQ")

		conn, err := amqp091.Dial(rabbitURL)
		if err != nil {
			log.Warn().
				Err(err).
				Int("attempt", attempt).
				Int("max_retries", maxRetries).
				Msg("Failed to connect to RabbitMQ")

			if attempt < maxRetries {
				log.Info().
					Dur("retry_in", retryInterval).
					Msg("Retrying RabbitMQ connection")
				time.Sleep(retryInterval)
				continue
			}

			// Last attempt failed
			lastRabbitErr = err
			rabbitEnabled = false
			log.Error().
				Err(err).
				Msg("Could not connect to RabbitMQ after all retries. RabbitMQ disabled.")
			return
		}

		// Connection successful, attempt to open channel
		channel, err := conn.Channel()
		if err != nil {
			conn.Close()
			log.Warn().
				Err(err).
				Int("attempt", attempt).
				Msg("Failed to open RabbitMQ channel")

			if attempt < maxRetries {
				log.Info().
					Dur("retry_in", retryInterval).
					Msg("Retrying RabbitMQ connection")
				time.Sleep(retryInterval)
				continue
			}

			// Last attempt failed
			lastRabbitErr = err
			rabbitEnabled = false
			log.Error().
				Err(err).
				Msg("Could not open RabbitMQ channel after all retries. RabbitMQ disabled.")
			return
		}

		// Success!
		lastRabbitErr = nil
		rabbitConn = conn
		rabbitChannel = channel
		rabbitEnabled = true

		log.Info().
			Str("queue", rabbitQueue).
			Int("attempt", attempt).
			Msg("RabbitMQ connection established successfully")

		// Setup handler for automatic reconnection on errors
		go handleConnectionErrors()
		return
	}
}

// Monitor connection errors and attempt reconnection
func handleConnectionErrors() {
	notifyClose := rabbitConn.NotifyClose(make(chan *amqp091.Error))

	for err := range notifyClose {
		log.Error().
			Err(err).
			Msg("RabbitMQ connection closed unexpectedly. Attempting reconnection...")

		lastRabbitErr = err
		rabbitEnabled = false

		// Attempt to reconnect
		var reconnErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			log.Info().
				Int("attempt", attempt).
				Msg("Reconnecting to RabbitMQ")

			time.Sleep(retryInterval)

			rabbitURL := os.Getenv("RABBITMQ_URL")
			conn, dialErr := amqp091.Dial(rabbitURL)
			if dialErr != nil {
				reconnErr = dialErr
				log.Warn().
					Err(dialErr).
					Int("attempt", attempt).
					Msg("Reconnection failed")
				continue
			}

			channel, chErr := conn.Channel()
			if chErr != nil {
				conn.Close()
				reconnErr = chErr
				log.Warn().
					Err(chErr).
					Int("attempt", attempt).
					Msg("Failed to open channel on reconnection")
				continue
			}

			// Reconnection successful
			lastRabbitErr = nil
			rabbitConn = conn
			rabbitChannel = channel
			rabbitEnabled = true

			log.Info().Msg("RabbitMQ reconnected successfully")

			// Restart monitoring
			go handleConnectionErrors()
			return
		}

		if reconnErr != nil {
			lastRabbitErr = reconnErr
		}
		log.Error().Msg("Failed to reconnect to RabbitMQ after all retries")
		return
	}
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
		// Check if RabbitMQ is configured but disabled due to connection issues
		rabbitURL := os.Getenv("RABBITMQ_URL")
		rabbitQueueEnv := os.Getenv("RABBITMQ_QUEUE")

		if rabbitURL != "" || rabbitQueueEnv != "" {
			urlSet := "no"
			if rabbitURL != "" {
				urlSet = "yes"
			}
			queueSet := "no"
			if rabbitQueueEnv != "" {
				queueSet = "yes"
			}
			ev := log.Error().
				Str("rabbitmq_url_set", urlSet).
				Str("rabbitmq_queue_set", queueSet)
			if lastRabbitErr != nil {
				ev = ev.Err(lastRabbitErr)
			}
			ev.Msg("RabbitMQ is configured but disabled due to connection failure. Event not published to queue.")
		} else {
			log.Debug().Msg("RabbitMQ not configured. Event not published to queue.")
		}
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
	originalData["serverId"] = queueServerID

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

func PublishFileErrorToQueue(payload WebhookFileErrorPayload) {

	queueName := *webhookErrorQueueName

	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal file error payload for RabbitMQ")
		return
	}

	err = PublishToRabbit(body, queueName)
	if err != nil {
		log.Error().Str("queue", queueName).Msg("Failed to publish file error payload to queue")
	} else {
		log.Info().Str("queue", queueName).Msg("File error payload successfully published to queue")
	}
}

func PublishDataErrorToQueue(payload WebhookErrorPayload) {
	queueName := *webhookErrorQueueName
	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal data error payload for RabbitMQ")
		return
	}
	err = PublishToRabbit(body, queueName)
	if err != nil {
		log.Error().Str("queue", queueName).Msg("Failed to publish data error payload to queue")
	} else {
		log.Info().Str("queue", queueName).Msg("Data error payload successfully published to queue")
	}
}
