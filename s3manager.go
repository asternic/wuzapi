package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// S3Config holds S3 configuration for a user
type S3Config struct {
	Enabled       bool
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	PathStyle     bool
	PublicURL     string
	MediaDelivery string
	RetentionDays int
}

// S3Manager manages S3 operations
type S3Manager struct {
	mu      sync.RWMutex
	db      *sqlx.DB
	clients map[string]*s3.Client
	configs map[string]*S3Config
}

// Global S3 manager instance
var s3Manager = &S3Manager{
	clients: make(map[string]*s3.Client),
	configs: make(map[string]*S3Config),
}

// GetS3Manager returns the global S3 manager instance
func GetS3Manager() *S3Manager {
	return s3Manager
}

// SetDB sets the database reference for lazy S3 client initialization
func (m *S3Manager) SetDB(db *sqlx.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
}

// EnsureClientFromDB loads S3 config from DB and initializes client if enabled. Returns true if client is available.
func (m *S3Manager) EnsureClientFromDB(userID string) bool {
	if _, _, ok := m.GetClient(userID); ok {
		return true
	}
	m.mu.RLock()
	db := m.db
	m.mu.RUnlock()
	if db == nil {
		return false
	}
	var s3DbConfig struct {
		Enabled       bool   `db:"s3_enabled"`
		Endpoint      string `db:"s3_endpoint"`
		Region        string `db:"s3_region"`
		Bucket        string `db:"s3_bucket"`
		AccessKey     string `db:"s3_access_key"`
		SecretKey     string `db:"s3_secret_key"`
		PathStyle     bool   `db:"s3_path_style"`
		PublicURL     string `db:"s3_public_url"`
		MediaDelivery string `db:"media_delivery"`
		RetentionDays int    `db:"s3_retention_days"`
	}
	query := `SELECT s3_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key, s3_secret_key, s3_path_style, s3_public_url, COALESCE(media_delivery, 'base64') AS media_delivery, COALESCE(s3_retention_days, 30) AS s3_retention_days FROM users WHERE id = $1`
	query = db.Rebind(query)
	if err := db.Get(&s3DbConfig, query, userID); err != nil || !s3DbConfig.Enabled {
		return false
	}
	config := &S3Config{
		Enabled:       s3DbConfig.Enabled,
		Endpoint:      s3DbConfig.Endpoint,
		Region:        s3DbConfig.Region,
		Bucket:        s3DbConfig.Bucket,
		AccessKey:     s3DbConfig.AccessKey,
		SecretKey:     s3DbConfig.SecretKey,
		PathStyle:     s3DbConfig.PathStyle,
		PublicURL:     s3DbConfig.PublicURL,
		MediaDelivery: s3DbConfig.MediaDelivery,
		RetentionDays: s3DbConfig.RetentionDays,
	}
	return m.InitializeS3Client(userID, config) == nil
}

// InitializeS3Client creates or updates S3 client for a user
func (m *S3Manager) InitializeS3Client(userID string, config *S3Config) error {
	if !config.Enabled {
		m.RemoveClient(userID)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Create custom credentials provider
	credProvider := credentials.NewStaticCredentialsProvider(
		config.AccessKey,
		config.SecretKey,
		"",
	)

	// Configure S3 client
	cfg := aws.Config{
		Region:      config.Region,
		Credentials: credProvider,
	}

	if config.Endpoint != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if service == s3.ServiceID {
				return aws.Endpoint{
					URL:               config.Endpoint,
					HostnameImmutable: config.PathStyle,
				}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		})
		cfg.EndpointResolverWithOptions = customResolver
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = config.PathStyle
	})

	m.clients[userID] = client
	m.configs[userID] = config

	log.Info().Str("userID", userID).Str("bucket", config.Bucket).Msg("S3 client initialized")
	return nil
}

// RemoveClient removes S3 client for a user
func (m *S3Manager) RemoveClient(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.clients, userID)
	delete(m.configs, userID)
}

// GetClient returns S3 client for a user
func (m *S3Manager) GetClient(userID string) (*s3.Client, *S3Config, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, clientOk := m.clients[userID]
	config, configOk := m.configs[userID]

	return client, config, clientOk && configOk
}

// GenerateS3Key generates S3 object key based on message metadata
func (m *S3Manager) GenerateS3Key(userID, contactJID, messageID string, mimeType string, isIncoming bool) string {
	// Determine direction
	direction := "outbox"
	if isIncoming {
		direction = "inbox"
	}

	// Clean contact JID
	contactJID = strings.ReplaceAll(contactJID, "@", "_")
	contactJID = strings.ReplaceAll(contactJID, ":", "_")

	// Get current time
	now := time.Now()
	year := now.Format("2025")
	month := now.Format("05")
	day := now.Format("25")

	// Determine media type folder
	mediaType := "documents"
	if strings.HasPrefix(mimeType, "image/") {
		mediaType = "images"
	} else if strings.HasPrefix(mimeType, "video/") {
		mediaType = "videos"
	} else if strings.HasPrefix(mimeType, "audio/") {
		mediaType = "audio"
	}

	// Get file extension
	ext := ".bin"
	switch {
	case strings.Contains(mimeType, "jpeg"), strings.Contains(mimeType, "jpg"):
		ext = ".jpg"
	case strings.Contains(mimeType, "png"):
		ext = ".png"
	case strings.Contains(mimeType, "gif"):
		ext = ".gif"
	case strings.Contains(mimeType, "webp"):
		ext = ".webp"
	case strings.Contains(mimeType, "mp4"):
		ext = ".mp4"
	case strings.Contains(mimeType, "webm"):
		ext = ".webm"
	case strings.Contains(mimeType, "ogg"):
		ext = ".ogg"
	case strings.Contains(mimeType, "opus"):
		ext = ".opus"
	case strings.Contains(mimeType, "pdf"):
		ext = ".pdf"
	case strings.Contains(mimeType, "spreadsheetml"):
		ext = ".xlsx"
	case strings.Contains(mimeType, "excel"):
		ext = ".xls"
	case strings.Contains(mimeType, "doc"):
		if strings.Contains(mimeType, "docx") {
			ext = ".docx"
		} else {
			ext = ".doc"
		}
	}

	// Build S3 key
	key := fmt.Sprintf("users/%s/%s/%s/%s/%s/%s/%s/%s%s",
		userID,
		direction,
		contactJID,
		year,
		month,
		day,
		mediaType,
		messageID,
		ext,
	)

	return key
}

// UploadToS3 uploads file to S3 and returns the key
func (m *S3Manager) UploadToS3(ctx context.Context, userID string, key string, data []byte, mimeType string) error {
	client, config, ok := m.GetClient(userID)
	if !ok {
		// Try lazy init from DB if available (handles reconnect-after-restart)
		if m.EnsureClientFromDB(userID) {
			client, config, ok = m.GetClient(userID)
		}
		if !ok {
			return fmt.Errorf("S3 client not initialized for user %s", userID)
		}
	}

	// Set content type and cache headers for preview
	contentType := mimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Calculate expiration time based on retention days
	var expires *time.Time
	if config.RetentionDays > 0 {
		expirationTime := time.Now().Add(time.Duration(config.RetentionDays) * 24 * time.Hour)
		expires = &expirationTime
	}

	input := &s3.PutObjectInput{
		Bucket:       aws.String(config.Bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String("public, max-age=3600"),
		ACL:          types.ObjectCannedACLPublicRead,
	}

	if expires != nil {
		input.Expires = expires
	}

	// Add content disposition for inline preview
	if strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "video/") || mimeType == "application/pdf" {
		input.ContentDisposition = aws.String("inline")
	}

	_, err := client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

// GetPublicURL generates public URL for S3 object
func (m *S3Manager) GetPublicURL(userID, key string) string {
	_, config, ok := m.GetClient(userID)
	if !ok {
		return ""
	}

	// Use custom public URL if configured
	if config.PublicURL != "" {
		return fmt.Sprintf("%s/%s/%s", strings.TrimRight(config.PublicURL, "/"), config.Bucket, key)
	}

	// Generate standard S3 URL
	if config.PathStyle {
		return fmt.Sprintf("%s/%s/%s",
			strings.TrimRight(config.Endpoint, "/"),
			config.Bucket,
			key)
	}

	// Virtual hosted-style URL
	if strings.Contains(config.Endpoint, "amazonaws.com") {
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s",
			config.Bucket,
			config.Region,
			key)
	}

	// For other S3-compatible services
	endpoint := strings.TrimPrefix(config.Endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	return fmt.Sprintf("https://%s.%s/%s", config.Bucket, endpoint, key)
}

// TestConnection tests S3 connection
func (m *S3Manager) TestConnection(ctx context.Context, userID string) error {
	client, config, ok := m.GetClient(userID)
	if !ok {
		return fmt.Errorf("S3 client not initialized for user %s", userID)
	}

	// Try to list objects with max 1 result
	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(config.Bucket),
		MaxKeys: aws.Int32(1),
	}

	_, err := client.ListObjectsV2(ctx, input)
	return err
}

// ProcessMediaForS3 handles the complete media upload process
func (m *S3Manager) ProcessMediaForS3(ctx context.Context, userID, contactJID, messageID string,
	data []byte, mimeType string, fileName string, isIncoming bool) (map[string]interface{}, error) {

	// Generate S3 key
	key := m.GenerateS3Key(userID, contactJID, messageID, mimeType, isIncoming)

	// Upload to S3
	err := m.UploadToS3(ctx, userID, key, data, mimeType)
	if err != nil {
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Generate public URL
	publicURL := m.GetPublicURL(userID, key)

	// Return S3 metadata
	s3Data := map[string]interface{}{
		"url":      publicURL,
		"key":      key,
		"bucket":   m.configs[userID].Bucket,
		"size":     len(data),
		"mimeType": mimeType,
		"fileName": fileName,
	}

	return s3Data, nil
}

// DeleteAllUserObjects deletes all user files from S3
func (m *S3Manager) DeleteAllUserObjects(ctx context.Context, userID string) error {
	client, config, ok := m.GetClient(userID)
	if !ok {
		return fmt.Errorf("S3 client not initialized for user %s", userID)
	}

	prefix := fmt.Sprintf("users/%s/", userID)
	var toDelete []types.ObjectIdentifier
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(config.Bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		}
		output, err := client.ListObjectsV2(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to list objects for user %s: %w", userID, err)
		}

		for _, obj := range output.Contents {
			toDelete = append(toDelete, types.ObjectIdentifier{Key: obj.Key})
			// Delete in batches of 1000 (S3 limit)
			if len(toDelete) == 1000 {
				_, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
					Bucket: aws.String(config.Bucket),
					Delete: &types.Delete{Objects: toDelete},
				})
				if err != nil {
					return fmt.Errorf("failed to delete objects for user %s: %w", userID, err)
				}
				toDelete = nil
			}
		}

		if output.IsTruncated != nil && *output.IsTruncated && output.NextContinuationToken != nil {
			continuationToken = output.NextContinuationToken
		} else {
			break
		}
	}

	// Delete any remaining objects
	if len(toDelete) > 0 {
		_, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(config.Bucket),
			Delete: &types.Delete{Objects: toDelete},
		})
		if err != nil {
			return fmt.Errorf("failed to delete objects for user %s: %w", userID, err)
		}
	}

	log.Info().Str("userID", userID).Msg("all user files removed from S3")
	return nil
}
