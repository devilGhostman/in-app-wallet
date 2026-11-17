package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"order-svc/internal/config"
	"order-svc/internal/repository"

	amqp "github.com/Azure/go-amqp"
	"go.uber.org/zap"
)

// OutboxPublisher polls unpaid outbox rows and publishes wallet credit commands.
type OutboxPublisher struct {
	cfg  config.RabbitMQConfig
	repo repository.OrderRepository
	log  *zap.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewOutboxPublisher(cfg config.RabbitMQConfig, repo repository.OrderRepository, log *zap.Logger) *OutboxPublisher {
	return &OutboxPublisher{cfg: cfg, repo: repo, log: log}
}

func (p *OutboxPublisher) Start(parent context.Context) {
	if !p.cfg.Enabled {
		p.log.Info("RabbitMQ outbox publisher disabled")
		return
	}
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.loop(ctx)
	}()
	p.log.Info("RabbitMQ outbox publisher started",
		zap.String("exchange", p.cfg.Exchange),
		zap.Duration("poll_interval", p.cfg.PollInterval),
	)
}

func (p *OutboxPublisher) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	p.log.Info("RabbitMQ outbox publisher stopped")
}

func (p *OutboxPublisher) loop(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := p.publishBatch(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			p.log.Error("outbox publish cycle failed; backing off",
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
			continue
		}
		backoff = time.Second
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.cfg.PollInterval):
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) error {
	rows, err := p.repo.ListUnpublishedOutbox(ctx, 50)
	if err != nil {
		return fmt.Errorf("list outbox: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	conn, err := dialAMQP(ctx, p.cfg.URL)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	session, err := conn.NewSession(ctx, nil)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	defer func() { _ = session.Close(context.Background()) }()

	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cmd, err := CommandFromOutboxPayload(row.Payload)
		if err != nil {
			p.log.Error("invalid outbox payload", zap.String("outbox_id", row.ID.String()), zap.Error(err))
			continue
		}
		if cmd.UserID == "" || cmd.OrderID == "" {
			p.log.Error("outbox payload missing user_id or order_id", zap.String("outbox_id", row.ID.String()))
			continue
		}
		body, err := json.Marshal(cmd)
		if err != nil {
			return err
		}
		addr := exchangeAddress(p.cfg.Exchange, cmd.UserID)
		sendCtx, cancel := context.WithTimeout(ctx, p.cfg.PublishTimeout)
		sender, err := session.NewSender(sendCtx, addr, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("sender %s: %w", addr, err)
		}
		err = sender.Send(sendCtx, &amqp.Message{
			Data: [][]byte{body},
			Properties: &amqp.MessageProperties{
				ContentType: strPtr("application/json"),
				Subject:     strPtr(cmd.UserID),
			},
		}, nil)
		_ = sender.Close(sendCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("send %s: %w", addr, err)
		}
		if err := p.repo.MarkOutboxPublished(ctx, row.ID, time.Now().UTC()); err != nil {
			return fmt.Errorf("mark published: %w", err)
		}
		p.log.Info("published wallet command",
			zap.String("outbox_id", row.ID.String()),
			zap.String("order_id", cmd.OrderID),
			zap.String("user_id", cmd.UserID),
			zap.String("reason", cmd.Reason),
			zap.String("address", addr),
		)
	}
	return nil
}

func exchangeAddress(exchange, routingKey string) string {
	ex := strings.TrimSpace(exchange)
	rk := strings.TrimSpace(routingKey)
	if strings.HasPrefix(ex, "/") {
		return strings.TrimSuffix(ex, "/") + "/" + url.PathEscape(rk)
	}
	return "/exchanges/" + ex + "/" + url.PathEscape(rk)
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

func strPtr(s string) *string { return &s }
