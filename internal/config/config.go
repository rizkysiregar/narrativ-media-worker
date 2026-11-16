// Package config loads worker settings from environment variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	Rabbit         amqp.URI
	RequestQueue   string
	ResultExchange string
	ResultRouting  string

	MinioEndpoint string
	MinioSecure   bool
	MinioAccess   string
	MinioSecret   string
	MinioBucket   string

	FFmpegPath       string
	Concurrency      int
	MaxAttempts      int
	ProcessTimeout   time.Duration
	HealthListenAddr string
}

// Load reads the configuration. Queue and exchange names must match the topology
// declared by narrativ-service (RabbitMQConfig); the worker never declares them.
func Load() (Config, error) {
	c := Config{
		RequestQueue:     env("VIDEO_THUMBNAIL_REQUEST_QUEUE", "video.thumbnail.request.queue"),
		ResultExchange:   env("RESULT_EXCHANGE", "post.exchange"),
		ResultRouting:    env("RESULT_ROUTING_KEY", "video.thumbnail.generated"),
		FFmpegPath:       env("FFMPEG_PATH", "ffmpeg"),
		HealthListenAddr: env("HEALTH_ADDR", ":8080"),
	}

	user, err := required("RABBITMQ_USERNAME")
	if err != nil {
		return c, err
	}
	pass, err := required("RABBITMQ_PASSWORD")
	if err != nil {
		return c, err
	}
	port, err := intEnv("RABBITMQ_PORT", 5672)
	if err != nil {
		return c, err
	}
	c.Rabbit = amqp.URI{
		Scheme:   "amqp",
		Host:     env("RABBITMQ_HOST", "localhost"),
		Port:     port,
		Username: user,
		Password: pass,
		Vhost:    env("RABBITMQ_VIRTUAL_HOST", "/narrativ"),
	}

	endpoint, err := required("MINIO_ENDPOINT")
	if err != nil {
		return c, err
	}
	c.MinioEndpoint, c.MinioSecure = parseEndpoint(endpoint)
	if c.MinioAccess, err = required("MINIO_ACCESS_KEY"); err != nil {
		return c, err
	}
	if c.MinioSecret, err = required("MINIO_SECRET_KEY"); err != nil {
		return c, err
	}
	if c.MinioBucket, err = required("MINIO_BUCKET"); err != nil {
		return c, err
	}

	if c.Concurrency, err = intEnv("WORKER_CONCURRENCY", 2); err != nil {
		return c, err
	}
	if c.MaxAttempts, err = intEnv("WORKER_MAX_ATTEMPTS", 3); err != nil {
		return c, err
	}
	timeout, err := intEnv("PROCESS_TIMEOUT_SECONDS", 120)
	if err != nil {
		return c, err
	}
	c.ProcessTimeout = time.Duration(timeout) * time.Second
	return c, nil
}

// parseEndpoint accepts "http://host:9000", "https://host" or a bare "host:9000".
func parseEndpoint(raw string) (host string, secure bool) {
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil {
			return u.Host, u.Scheme == "https"
		}
	}
	return raw, false
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func required(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("missing required env %s", key)
	}
	return v, nil
}

func intEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("env %s must be a positive integer, got %q", key, v)
	}
	return n, nil
}
