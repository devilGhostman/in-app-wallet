package handler

import (
	"encoding/json"
	"io"

	"order-svc/internal/dto"
	"order-svc/internal/middleware"
	"order-svc/internal/services"
	"order-svc/pkg/response"

	"github.com/gin-gonic/gin"
)

type OrderHandler struct {
	orders *services.OrderService
	pay    *services.PaymentSimulator
	secret string
}

func NewOrderHandler(orders *services.OrderService, pay *services.PaymentSimulator, webhookSecret string) *OrderHandler {
	return &OrderHandler{orders: orders, pay: pay, secret: webhookSecret}
}

// Create godoc
// @Summary Create a purchase order
// @Tags orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT"
// @Param X-Scopes header string true "order:create"
// @Param X-App-Id header string true "Application id"
// @Param body body dto.CreateOrderRequest true "order"
// @Success 201 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /internal/orders [post]
func (h *OrderHandler) Create(c *gin.Context) {
	var req dto.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "Invalid request body", err.Error()))
		return
	}
	out, err := h.orders.Create(c.Request.Context(), middleware.GetAppID(c), req)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Created(c, out, "Order created")
}

// Spend godoc
// @Summary Create a spend order (queues wallet SPEND debit; no payment gateway)
// @Tags orders
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT"
// @Param X-Scopes header string true "order:spend"
// @Param X-App-Id header string true "Application id"
// @Param body body dto.SpendOrderRequest true "spend"
// @Success 201 {object} response.SuccessEnvelope
// @Security BearerAuth
// @Router /internal/orders/spend [post]
func (h *OrderHandler) Spend(c *gin.Context) {
	var req dto.SpendOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "Invalid request body", err.Error()))
		return
	}
	out, err := h.orders.CreateSpend(c.Request.Context(), middleware.GetAppID(c), req)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Created(c, out, "Spend order created")
}

// Get godoc
// @Summary Get order by id
// @Tags orders
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT"
// @Param X-Scopes header string true "order:read"
// @Param X-App-Id header string true "Application id"
// @Param order_id path string true "order id"
// @Success 200 {object} response.SuccessEnvelope
// @Security BearerAuth
// @Router /internal/orders/{order_id} [get]
func (h *OrderHandler) Get(c *gin.Context) {
	out, err := h.orders.Get(c.Request.Context(), middleware.GetAppID(c), c.Param("order_id"))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out)
}

// SimulatePayment godoc
// @Summary Fake payment gateway — POSTs signed webhook
// @Tags payments
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT"
// @Param X-Scopes header string true "order:pay_simulate"
// @Param X-App-Id header string true "Application id"
// @Param body body dto.SimulatePaymentRequest true "simulate"
// @Success 200 {object} response.SuccessEnvelope
// @Security BearerAuth
// @Router /internal/payments/simulate [post]
func (h *OrderHandler) SimulatePayment(c *gin.Context) {
	var req dto.SimulatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "Invalid request body", err.Error()))
		return
	}
	out, err := h.pay.Simulate(c.Request.Context(), middleware.GetAppID(c), req)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out, "Payment simulated")
}

// PaymentWebhook godoc
// @Summary Payment webhook (HMAC X-Payment-Signature)
// @Tags webhooks
// @Accept json
// @Produce json
// @Param X-Payment-Signature header string true "hex HMAC-SHA256 of raw body"
// @Param body body dto.PaymentWebhookRequest true "webhook"
// @Success 200 {object} response.SuccessEnvelope
// @Router /webhooks/payment [post]
func (h *OrderHandler) PaymentWebhook(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "failed to read body"))
		return
	}
	sig := c.GetHeader("X-Payment-Signature")
	if !services.VerifyWebhookSignature(h.secret, sig, body) {
		response.Error(c, response.Unauthorized(response.CodeNotAuthorized, "invalid payment signature"))
		return
	}
	var req dto.PaymentWebhookRequest
	if err := json.Unmarshal(body, &req); err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "Invalid request body", err.Error()))
		return
	}
	out, err := h.orders.MarkPaidFromWebhook(c.Request.Context(), req)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out, "Payment recorded")
}
