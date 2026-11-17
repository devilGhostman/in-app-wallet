package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"wallet-svc/internal/config"

	amqp "github.com/Azure/go-amqp"
	"go.uber.org/zap"
)

// Consumer runs AMQP 1.0 receive loops for assigned consistent-hash shard queues.
type Consumer struct {
	cfg     config.RabbitMQConfig
	handler MutationHandler
	log     *zap.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewConsumer(cfg config.RabbitMQConfig, handler MutationHandler, log *zap.Logger) *Consumer {
	return &Consumer{cfg: cfg, handler: handler, log: log}
}

func (c *Consumer) Start(parent context.Context) {
	if !c.cfg.Enabled {
		c.log.Info("RabbitMQ consumers disabled")
		return
	}
	queues, err := AssignedQueues(c.cfg)
	if err != nil {
		c.log.Error("RabbitMQ shard assignment failed; consumers not started", zap.Error(err))
		return
	}
	if len(queues) == 0 {
		c.log.Warn("RabbitMQ shard assignment empty; consumers not started",
			zap.Int("shard_total", c.cfg.ShardTotal),
			zap.Int("shard_index", c.cfg.ShardIndex),
			zap.Int("shard_count", c.cfg.ShardCount),
		)
		return
	}

	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel

	c.wg.Add(len(queues))
	for _, q := range queues {
		queue := q
		go func() {
			defer c.wg.Done()
			c.runQueue(ctx, queue)
		}()
	}
	c.log.Info("RabbitMQ consumers started",
		zap.String("exchange", c.cfg.Exchange),
		zap.String("queue_prefix", c.cfg.QueuePrefix),
		zap.Int("shard_total", c.cfg.ShardTotal),
		zap.Int("shard_index", c.cfg.ShardIndex),
		zap.Int("shard_count", c.cfg.ShardCount),
		zap.Strings("queues", queues),
		zap.String("dlq", c.cfg.CommandsDLQ),
		zap.Int("prefetch", c.cfg.Prefetch),
	)
}

func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	c.log.Info("RabbitMQ consumers stopped")
}

func (c *Consumer) runQueue(ctx context.Context, queue string) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.consumeLoop(ctx, queue)
		if ctx.Err() != nil {
			return
		}
		c.log.Error("RabbitMQ consumer disconnected; reconnecting",
			zap.String("queue", queue),
			zap.Error(err),
			zap.Duration("backoff", backoff),
		)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
		backoff += time.Duration(rand.Intn(250)) * time.Millisecond
	}
}

func (c *Consumer) consumeLoop(ctx context.Context, queue string) error {
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

	addr := queueAddress(queue)
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

	dlq := c.cfg.CommandsDLQ
	dlqAddr := queueAddress(dlq)
	dlqSender, err := session.NewSender(ctx, dlqAddr, nil)
	if err != nil {
		return fmt.Errorf("dlq sender %s: %w", dlqAddr, err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = dlqSender.Close(closeCtx)
	}()

	c.log.Info("RabbitMQ receiver attached",
		zap.String("address", addr),
		zap.String("dlq", dlqAddr),
		zap.String("events_exchange", c.cfg.EventsExchange),
	)

	for {
		msg, err := receiver.Receive(ctx, nil)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("receive: %w", err)
		}
		c.handleDelivery(ctx, session, receiver, dlqSender, msg, queue, dlq)
	}
}

func (c *Consumer) handleDelivery(
	ctx context.Context,
	session *amqp.Session,
	receiver *amqp.Receiver,
	dlqSender *amqp.Sender,
	msg *amqp.Message,
	queue, dlq string,
) {
	data := msg.GetData()
	cmd, err := ParseCommand(data)
	if err != nil {
		c.log.Error("invalid wallet command message",
			zap.String("queue", queue),
			zap.Error(err),
			zap.ByteString("body", data),
		)
		c.settle(ctx, session, receiver, dlqSender, msg, SettleDeadLetter, queue, dlq, "", nil, err)
		return
	}
	if err := cmd.Validate(); err != nil {
		c.log.Error("wallet command validation failed",
			zap.String("queue", queue),
			zap.String("order_id", cmd.OrderID),
			zap.Error(err),
		)
		c.settle(ctx, session, receiver, dlqSender, msg, SettleDeadLetter, queue, dlq, "", &cmd, err)
		return
	}
	op, _ := OpFromReason(cmd.Reason)

	out, err := c.handler.Handle(ctx, cmd)
	if err != nil {
		action := ClassifyProcessError(err)
		c.log.Error("wallet command processing failed",
			zap.String("queue", queue),
			zap.String("op", string(op)),
			zap.String("app_id", cmd.AppID),
			zap.String("user_id", cmd.UserID),
			zap.String("order_id", cmd.OrderID),
			zap.String("idempotency_key", cmd.IdempotencyKey),
			zap.String("settle", settleName(action)),
			zap.Error(err),
		)
		c.settle(ctx, session, receiver, dlqSender, msg, action, queue, dlq, op, &cmd, err)
		return
	}

	c.log.Info("wallet command processed",
		zap.String("queue", queue),
		zap.String("op", string(op)),
		zap.String("app_id", cmd.AppID),
		zap.String("user_id", cmd.UserID),
		zap.String("order_id", cmd.OrderID),
		zap.String("ledger_id", out.LedgerID),
		zap.String("transaction_id", out.TransactionID),
		zap.Bool("duplicate", out.Duplicate),
		zap.Int64("balance", out.Balance),
	)
	if err := receiver.AcceptMessage(ctx, msg); err != nil {
		c.log.Error("failed to accept message", zap.Error(err))
	}
}

func (c *Consumer) settle(
	ctx context.Context,
	session *amqp.Session,
	receiver *amqp.Receiver,
	dlqSender *amqp.Sender,
	msg *amqp.Message,
	action SettleAction,
	queue, dlq string,
	op MutationOp,
	cmd *WalletMutationCommand,
	cause error,
) {
	switch action {
	case SettleRelease:
		if relErr := receiver.ReleaseMessage(ctx, msg); relErr != nil {
			c.log.Error("failed to release message", zap.Error(relErr))
		}
	case SettleDeadLetter:
		if cmd != nil && strings.TrimSpace(cmd.OrderID) != "" {
			if pubErr := c.publishFailureEvent(ctx, session, *cmd, op, cause); pubErr != nil {
				c.log.Error("failed to publish wallet failure event; releasing for retry",
					zap.String("queue", queue),
					zap.String("order_id", cmd.OrderID),
					zap.Error(pubErr),
				)
				if relErr := receiver.ReleaseMessage(ctx, msg); relErr != nil {
					c.log.Error("failed to release message after failure-event publish error", zap.Error(relErr))
				}
				return
			}
		}
		if pubErr := publishDeadLetter(ctx, dlqSender, msg, queue, dlq, op, cause); pubErr != nil {
			c.log.Error("failed to publish dead-letter; releasing for retry",
				zap.String("queue", queue),
				zap.String("dlq", dlq),
				zap.Error(pubErr),
			)
			if relErr := receiver.ReleaseMessage(ctx, msg); relErr != nil {
				c.log.Error("failed to release message after dlq failure", zap.Error(relErr))
			}
			return
		}
		c.log.Warn("message moved to dead-letter queue",
			zap.String("queue", queue),
			zap.String("dlq", dlq),
			zap.String("op", string(op)),
			zap.Error(cause),
		)
		if accErr := receiver.AcceptMessage(ctx, msg); accErr != nil {
			c.log.Error("failed to accept message after dlq", zap.Error(accErr))
		}
	default:
		if accErr := receiver.AcceptMessage(ctx, msg); accErr != nil {
			c.log.Error("failed to accept message", zap.Error(accErr))
		}
	}
}

func (c *Consumer) publishFailureEvent(
	ctx context.Context,
	session *amqp.Session,
	cmd WalletMutationCommand,
	op MutationOp,
	cause error,
) error {
	ev, err := FailureEventFromCommand(cmd, op, cause)
	if err != nil {
		return err
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	addr := exchangeAddress(c.cfg.EventsExchange, ev.EventType)
	sendCtx, cancel := context.WithTimeout(ctx, c.cfg.PublishTimeout)
	defer cancel()
	sender, err := session.NewSender(sendCtx, addr, nil)
	if err != nil {
		return fmt.Errorf("failure event sender %s: %w", addr, err)
	}
	defer func() { _ = sender.Close(sendCtx) }()
	if err := sender.Send(sendCtx, &amqp.Message{
		Data: [][]byte{body},
		Properties: &amqp.MessageProperties{
			ContentType: strPtr("application/json"),
			Subject:     strPtr(ev.EventType),
		},
	}, nil); err != nil {
		return fmt.Errorf("send failure event %s: %w", addr, err)
	}
	c.log.Info("published wallet failure event",
		zap.String("event_type", ev.EventType),
		zap.String("order_id", ev.OrderID),
		zap.String("code", ev.Code),
		zap.String("address", addr),
	)
	return nil
}

func publishDeadLetter(
	ctx context.Context,
	sender *amqp.Sender,
	src *amqp.Message,
	queue, dlq string,
	op MutationOp,
	cause error,
) error {
	return sender.Send(ctx, buildDeadLetterMessage(src, queue, dlq, op, cause, time.Now().UTC()), nil)
}

func buildDeadLetterMessage(
	src *amqp.Message,
	queue, dlq string,
	op MutationOp,
	cause error,
	failedAt time.Time,
) *amqp.Message {
	props := map[string]any{
		"x-original-queue": queue,
		"x-dlq":            dlq,
		"x-op":             string(op),
		"x-failed-at":      failedAt.Format(time.RFC3339Nano),
	}
	if cause != nil {
		props["x-error"] = cause.Error()
	}
	for k, v := range src.ApplicationProperties {
		if _, exists := props[k]; !exists {
			props[k] = v
		}
	}

	out := &amqp.Message{
		Data:                  [][]byte{src.GetData()},
		ApplicationProperties: props,
	}
	if src.Properties != nil {
		out.Properties = &amqp.MessageProperties{
			MessageID:     src.Properties.MessageID,
			CorrelationID: src.Properties.CorrelationID,
			ContentType:   src.Properties.ContentType,
			Subject:       src.Properties.Subject,
		}
	}
	return out
}

func settleName(a SettleAction) string {
	switch a {
	case SettleRelease:
		return "release"
	case SettleDeadLetter:
		return "dead_letter"
	default:
		return "accept"
	}
}

func queueAddress(queue string) string {
	q := strings.TrimSpace(queue)
	if strings.HasPrefix(q, "/") {
		return q
	}
	return "/queues/" + q
}

func dialAMQP(ctx context.Context, rawURL string) (*amqp.Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse rabbitmq url: %w", err)
	}
	opts := &amqp.ConnOptions{}
	if u.User != nil {
		user := u.User.Username()
		pass, _ := u.User.Password()
		opts.SASLType = amqp.SASLTypePlain(user, pass)
		u.User = nil
	}
	return amqp.Dial(ctx, u.String(), opts)
}
