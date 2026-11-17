package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"order-svc/internal/config"
	"order-svc/internal/services"

	amqp "github.com/Azure/go-amqp"
	"go.uber.org/zap"
)

// LifecycleConsumer receives wallet.credited|debited|credit_failed|debit_failed.
type LifecycleConsumer struct {
	cfg    config.RabbitMQConfig
	orders *services.OrderService
	log    *zap.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type walletLifecycleEvent struct {
	EventType      string `json:"event_type"`
	AppID          string `json:"app_id"`
	UserID         string `json:"user_id"`
	OrderID        string `json:"order_id"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	Reason         string `json:"reason"`
}

func NewLifecycleConsumer(cfg config.RabbitMQConfig, orders *services.OrderService, log *zap.Logger) *LifecycleConsumer {
	return &LifecycleConsumer{cfg: cfg, orders: orders, log: log}
}

func (c *LifecycleConsumer) Start(parent context.Context) {
	if !c.cfg.Enabled {
		c.log.Info("RabbitMQ lifecycle consumer disabled")
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.run(ctx)
	}()
	c.log.Info("RabbitMQ lifecycle consumer started", zap.String("queue", c.cfg.EventsQueue))
}

func (c *LifecycleConsumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	c.log.Info("RabbitMQ lifecycle consumer stopped")
}

func (c *LifecycleConsumer) run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.consumeLoop(ctx)
		if ctx.Err() != nil {
			return
		}
		c.log.Error("lifecycle consumer disconnected; reconnecting", zap.Error(err), zap.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
		backoff += time.Duration(rand.Intn(250)) * time.Millisecond
	}
}

func (c *LifecycleConsumer) consumeLoop(ctx context.Context) error {
	conn, err := dialAMQP(ctx, c.cfg.URL)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	session, err := conn.NewSession(ctx, nil)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	defer func() { _ = session.Close(context.Background()) }()

	addr := queueAddress(c.cfg.EventsQueue)
	receiver, err := session.NewReceiver(ctx, addr, &amqp.ReceiverOptions{
		Credit: int32(c.cfg.Prefetch),
	})
	if err != nil {
		return fmt.Errorf("receiver %s: %w", addr, err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = receiver.Close(closeCtx)
	}()

	for {
		msg, err := receiver.Receive(ctx, nil)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("receive: %w", err)
		}
		c.handle(ctx, receiver, msg)
	}
}

func (c *LifecycleConsumer) handle(ctx context.Context, receiver *amqp.Receiver, msg *amqp.Message) {
	var ev walletLifecycleEvent
	if err := json.Unmarshal(msg.GetData(), &ev); err != nil {
		c.log.Error("invalid lifecycle event json", zap.Error(err), zap.ByteString("body", msg.GetData()))
		_ = receiver.AcceptMessage(ctx, msg)
		return
	}
	if ev.EventType == "" && msg.Properties != nil && msg.Properties.Subject != nil {
		ev.EventType = *msg.Properties.Subject
	}
	if err := c.orders.ApplyWalletLifecycleEvent(ctx, ev.EventType, ev.AppID, ev.OrderID, ev.Reason, ev.Code, ev.Message); err != nil {
		c.log.Error("failed to apply lifecycle event; releasing",
			zap.Error(err),
			zap.String("event", ev.EventType),
			zap.String("order_id", ev.OrderID),
			zap.String("reason", ev.Reason),
		)
		_ = receiver.ReleaseMessage(ctx, msg)
		return
	}
	if err := receiver.AcceptMessage(ctx, msg); err != nil {
		c.log.Error("failed to accept lifecycle message", zap.Error(err))
	}
}

func queueAddress(queue string) string {
	q := strings.TrimSpace(queue)
	if strings.HasPrefix(q, "/") {
		return q
	}
	return "/queues/" + q
}
