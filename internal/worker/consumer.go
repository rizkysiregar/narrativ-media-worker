package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/rizkysiregar/narrativ-media-worker/internal/config"
)

// Handler turns a request into a result.
type Handler interface {
	Process(ctx context.Context, req ThumbnailRequested) (ThumbnailGenerated, error)
}

// Consumer reads thumbnail requests from RabbitMQ and publishes the results.
//
// Delivery semantics are at-least-once: a request is acked only after the result
// was published and confirmed by the broker. Failed requests are rejected without
// requeue so the queue's dead-letter exchange (declared by narrativ-service)
// routes them to the DLQ.
type Consumer struct {
	cfg     config.Config
	handler Handler
	ready   atomic.Bool

	// backoff is the wait before retry number attempt (1-based); replaced in tests.
	backoff func(attempt int) time.Duration
}

func NewConsumer(cfg config.Config, handler Handler) *Consumer {
	return &Consumer{
		cfg:     cfg,
		handler: handler,
		backoff: func(attempt int) time.Duration {
			return min(time.Duration(1<<(attempt-1))*time.Second, 5*time.Second)
		},
	}
}

// Ready reports whether the consumer currently holds a live broker session.
func (c *Consumer) Ready() bool { return c.ready.Load() }

// Run keeps a broker session alive, reconnecting with backoff, until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	wait := time.Second
	for {
		err := c.session(ctx)
		c.ready.Store(false)
		if ctx.Err() != nil {
			return nil
		}
		slog.Error("rabbitmq session ended, reconnecting", "error", err, "in", wait)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		wait = min(wait*2, 30*time.Second)
	}
}

func (c *Consumer) session(ctx context.Context) error {
	conn, err := amqp.Dial(c.cfg.Rabbit.String())
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("enable publisher confirms: %w", err)
	}
	if err := ch.Qos(c.cfg.Concurrency, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	returns := ch.NotifyReturn(make(chan amqp.Return, 1))
	go func() {
		for r := range returns {
			slog.Error("result message was unroutable",
				"exchange", r.Exchange, "routingKey", r.RoutingKey, "replyText", r.ReplyText)
		}
	}()

	deliveries, err := ch.Consume(c.cfg.RequestQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", c.cfg.RequestQueue, err)
	}

	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	c.ready.Store(true)
	slog.Info("consuming", "queue", c.cfg.RequestQueue, "concurrency", c.cfg.Concurrency)

	var publishMu sync.Mutex
	var wg sync.WaitGroup
	for range c.cfg.Concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range deliveries {
				c.handle(ctx, ch, &publishMu, d)
			}
		}()
	}

	select {
	case <-ctx.Done():
		// Stop taking new work, let in-flight deliveries finish, then close.
		_ = ch.Cancel("", false)
		wg.Wait()
		return nil
	case e := <-closed:
		wg.Wait()
		return fmt.Errorf("connection closed: %v", e)
	}
}

func (c *Consumer) handle(ctx context.Context, ch *amqp.Channel, publishMu *sync.Mutex, d amqp.Delivery) {
	var req ThumbnailRequested
	if err := json.Unmarshal(d.Body, &req); err != nil {
		slog.Error("malformed message, sending to DLQ", "error", err)
		_ = d.Nack(false, false)
		return
	}

	res, err := c.processWithRetry(ctx, req)
	if err != nil {
		slog.Error("thumbnail failed, sending to DLQ", "postId", req.PostID, "error", err)
		_ = d.Nack(false, false)
		return
	}

	if err := c.publish(ctx, ch, publishMu, res); err != nil {
		slog.Error("publishing result failed, sending to DLQ", "postId", req.PostID, "error", err)
		_ = d.Nack(false, false)
		return
	}
	_ = d.Ack(false)
}

func (c *Consumer) processWithRetry(ctx context.Context, req ThumbnailRequested) (ThumbnailGenerated, error) {
	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.ProcessTimeout)
		res, err := c.handler.Process(attemptCtx, req)
		cancel()
		if err == nil {
			return res, nil
		}
		lastErr = err
		if errors.Is(err, ErrInvalid) || ctx.Err() != nil || attempt == c.cfg.MaxAttempts {
			break
		}
		slog.Warn("thumbnail attempt failed, retrying",
			"postId", req.PostID, "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ThumbnailGenerated{}, ctx.Err()
		case <-time.After(c.backoff(attempt)):
		}
	}
	return ThumbnailGenerated{}, lastErr
}

func (c *Consumer) publish(ctx context.Context, ch *amqp.Channel, mu *sync.Mutex, res ThumbnailGenerated) error {
	body, err := json.Marshal(res)
	if err != nil {
		return err
	}

	pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	mu.Lock()
	confirmation, err := ch.PublishWithDeferredConfirmWithContext(pubCtx,
		c.cfg.ResultExchange, c.cfg.ResultRouting, true, false,
		amqp.Publishing{
			// Spring's Jackson converter only decodes the body when the content type is JSON.
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    res.PostID,
			Timestamp:    time.Now(),
			Body:         body,
		})
	mu.Unlock()
	if err != nil {
		return err
	}

	acked, err := confirmation.WaitContext(pubCtx)
	if err != nil {
		return err
	}
	if !acked {
		return errors.New("broker nacked the result message")
	}
	return nil
}
