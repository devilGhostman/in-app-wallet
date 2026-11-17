package dto

type CreateOrderRequest struct {
	UserID         string         `json:"user_id" binding:"required"`
	Amount         int64          `json:"amount" binding:"required"`
	IdempotencyKey string         `json:"idempotency_key" binding:"required"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type SpendOrderRequest struct {
	UserID         string         `json:"user_id" binding:"required"`
	Amount         int64          `json:"amount" binding:"required"`
	IdempotencyKey string         `json:"idempotency_key" binding:"required"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type OrderResponse struct {
	OrderID        string         `json:"order_id"`
	AppID          string         `json:"app_id"`
	UserID         string         `json:"user_id"`
	Amount         int64          `json:"amount"`
	Type           string         `json:"type"`
	Status         string         `json:"status"`
	IdempotencyKey string         `json:"idempotency_key"`
	PaymentRef     string         `json:"payment_ref,omitempty"`
	FailureCode    string         `json:"failure_code,omitempty"`
	FailureMessage string         `json:"failure_message,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	PaidAt         string         `json:"paid_at,omitempty"`
	CreatedAt      string         `json:"created_at"`
}

type SimulatePaymentRequest struct {
	OrderID string `json:"order_id" binding:"required"`
}

type SimulatePaymentResponse struct {
	OrderID    string `json:"order_id"`
	PaymentRef string `json:"payment_ref"`
	Status     string `json:"status"`
	WebhookOK  bool   `json:"webhook_ok"`
}

type PaymentWebhookRequest struct {
	OrderID    string `json:"order_id" binding:"required"`
	PaymentRef string `json:"payment_ref" binding:"required"`
	Status     string `json:"status" binding:"required"`
	Amount     int64  `json:"amount" binding:"required"`
	AppID      string `json:"app_id" binding:"required"`
}
