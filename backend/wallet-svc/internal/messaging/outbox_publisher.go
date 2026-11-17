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

	"wallet-svc/internal/config"
	"wallet-svc/internal/repository"

	amqp "github.com/Azure/go-amqp"
	"go.uber.org/zap"
)

// OutboxPublisher polls wallet outbox rows (credited/debited) onto wallet.events.
type OutboxPublisher struct {
	cfg  config.RabbitMQConfig
	repo repository.WalletRepository
	log  *zap.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewOutboxPublisher(cfg config.RabbitMQConfig, repo repository.WalletRepository, log *zap.Logger) *OutboxPublisher {
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
		zap.String("events_exchange", p.cfg.EventsExchange),
		zap.Duration("poll_interval", p.cfg.OutboxPollInterval),
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
			p.log.Error("wallet outbox publish cycle failed; backing off",
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
		case <-time.After(p.cfg.OutboxPollInterval):
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
		ev := LifecycleEventFromOutbox(row.EventType, row.Payload)
		body, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		rk := row.EventType
		if rk == "" {
			rk = ev.EventType
		}
		addr := exchangeAddress(p.cfg.EventsExchange, rk)
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
				Subject:     strPtr(ev.EventType),
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
		p.log.Info("published wallet lifecycle event",
			zap.String("outbox_id", row.ID.String()),
			zap.String("event_type", ev.EventType),
			zap.String("order_id", ev.OrderID),
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

func strPtr(s string) *string { return &s }
