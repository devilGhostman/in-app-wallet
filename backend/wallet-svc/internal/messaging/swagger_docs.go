package messaging

// This file documents AMQP 1.0 contracts in Swagger only.
// Paths under /amqp/... are not HTTP routes; Orders publishes to RabbitMQ.

// CommandsExchangePublish godoc
// @Summary Publish wallet command via consistent-hash exchange (AMQP 1.0)
// @Description Orders publishes JSON to exchange wallet.commands (type x-consistent-hash) with routing key = user_id. AMQP 1.0 address: /exchanges/wallet.commands/{user_id}. Both credit and debit use this exchange; reason TOKEN_PURCHASE→Credit, SPEND→Debit. Same-user messages stay on one shard queue and are processed in publish order. No JWT; include app_id in the body.
// @Tags messaging
// @Accept json
// @Produce json
// @Param user_id path string true "user id (also the consistent-hash routing key)"
// @Param body body WalletMutationCommand true "credit or debit command"
// @Success 202 {object} map[string]string "accepted by broker (not an HTTP response)"
// @Failure 400 {object} map[string]string "invalid body → wallet.commands.dlq after consume"
// @Router /amqp/exchanges/wallet.commands/{user_id} [post]
func CommandsExchangePublish() {}

// ShardQueue godoc
// @Summary Shard command queue (AMQP 1.0)
// @Description Durable queues wallet.commands.0 … wallet.commands.{N-1} bound to wallet.commands with equal weight. wallet-svc consumes a subset via RABBITMQ_SHARD_INDEX / RABBITMQ_SHARD_COUNT (shardID % COUNT == INDEX). One consumer per shard among replicas; processed serially per queue.
// @Tags messaging
// @Accept json
// @Produce json
// @Param shard path int true "shard id 0..SHARD_TOTAL-1"
// @Param body body WalletMutationCommand true "routed command"
// @Success 200 {object} map[string]string "message available on shard queue"
// @Router /amqp/queues/wallet.commands/{shard} [get]
func ShardQueue() {}

// CommandsDLQ godoc
// @Summary Commands dead-letter queue (AMQP 1.0)
// @Description Durable queue wallet.commands.dlq (address /queues/wallet.commands.dlq). Body is the original WalletMutationCommand JSON. Application properties include x-original-queue, x-dlq, x-op, x-error, x-failed-at. Consumed manually / by ops tooling; wallet-svc does not reprocess DLQ automatically.
// @Tags messaging
// @Accept json
// @Produce json
// @Param body body WalletMutationCommand true "original failed command"
// @Success 200 {object} map[string]string "message available on DLQ"
// @Router /amqp/queues/wallet.commands.dlq [get]
func CommandsDLQ() {}

// LifecycleEventsExchange godoc
// @Summary Publish wallet lifecycle event (AMQP 1.0)
// @Description Topic exchange wallet.events. Success events wallet.credited / wallet.debited come from the DB outbox publisher. Permanent settle publishes wallet.credit_failed / wallet.debit_failed before DLQ. Routing key = event_type. Bound queue order.wallet.lifecycle (RK wallet.*) is consumed by order-svc.
// @Tags messaging
// @Accept json
// @Produce json
// @Param event_type path string true "wallet.credited|wallet.debited|wallet.credit_failed|wallet.debit_failed"
// @Param body body LifecycleEvent true "lifecycle event"
// @Success 202 {object} map[string]string "accepted by broker (not an HTTP response)"
// @Router /amqp/exchanges/wallet.events/{event_type} [post]
func LifecycleEventsExchange() {}
