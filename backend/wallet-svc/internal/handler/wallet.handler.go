package handler

import (
	"strconv"

	"wallet-svc/internal/dto"
	"wallet-svc/internal/middleware"
	"wallet-svc/internal/services"
	"wallet-svc/pkg/response"

	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	svc *services.WalletService
}

func NewWalletHandler(svc *services.WalletService) *WalletHandler {
	return &WalletHandler{svc: svc}
}

// Create godoc
// @Summary Create wallet
// @Tags wallet
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param body body dto.CreateWalletRequest true "user"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /internal/wallets [post]
func (h *WalletHandler) Create(c *gin.Context) {
	var req dto.CreateWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "Invalid request body", err.Error()))
		return
	}
	out, err := h.svc.Create(c.Request.Context(), middleware.GetAppID(c), req.UserID)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out, "Wallet ready")
}

// Get godoc
// @Summary Get wallet balance
// @Tags wallet
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param user_id path string true "user id"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /internal/wallets/{user_id} [get]
func (h *WalletHandler) Get(c *gin.Context) {
	out, err := h.svc.Get(c.Request.Context(), middleware.GetAppID(c), c.Param("user_id"))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out)
}

// Credit godoc
// @Summary Credit coins after verified payment (CREDIT user + DEBIT app)
// @Description HTTP equivalent of AMQP publish to exchange wallet.commands (RK=user_id; see messaging tag). reason must be TOKEN_PURCHASE; idempotency_key order:{order_id}:credit.
// @Tags wallet
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param user_id path string true "user id"
// @Param body body dto.MutationRequest true "credit"
// @Success 201 {object} response.SuccessEnvelope
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /internal/wallets/{user_id}/credit [post]
func (h *WalletHandler) Credit(c *gin.Context) {
	h.applyMutation(c, true)
}

// Debit godoc
// @Summary Debit coins for a content spend (DEBIT user + CREDIT app)
// @Description HTTP equivalent of AMQP publish to exchange wallet.commands (RK=user_id; see messaging tag). reason must be SPEND; idempotency_key order:{order_id}:debit.
// @Tags wallet
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param user_id path string true "user id"
// @Param body body dto.MutationRequest true "debit"
// @Success 201 {object} response.SuccessEnvelope
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /internal/wallets/{user_id}/debit [post]
func (h *WalletHandler) Debit(c *gin.Context) {
	h.applyMutation(c, false)
}

func (h *WalletHandler) applyMutation(c *gin.Context, credit bool) {
	var req dto.MutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "Invalid request body", err.Error()))
		return
	}
	requestID := middleware.GetRequestID(c)
	appID := middleware.GetAppID(c)
	var (
		out    dto.MutationResponse
		status int
		err    error
	)
	if credit {
		out, status, err = h.svc.Credit(c.Request.Context(), appID, c.Param("user_id"), requestID, req)
	} else {
		out, status, err = h.svc.Debit(c.Request.Context(), appID, c.Param("user_id"), requestID, req)
	}
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.JSON(c, status, out)
}

// FundApp godoc
// @Summary Fund the app coin pool (CREDIT app + DEBIT mint)
// @Tags wallet
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param body body dto.FundAppRequest true "fund"
// @Success 201 {object} response.SuccessEnvelope
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /internal/apps/fund [post]
func (h *WalletHandler) FundApp(c *gin.Context) {
	var req dto.FundAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "Invalid request body", err.Error()))
		return
	}
	out, status, err := h.svc.FundApp(c.Request.Context(), middleware.GetAppID(c), middleware.GetRequestID(c), req)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.JSON(c, status, out)
}

// Ledger godoc
// @Summary List ledger entries
// @Tags wallet
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param user_id path string true "user id"
// @Param cursor query string false "cursor"
// @Param limit query int false "limit"
// @Param entry_type query string false "CREDIT or DEBIT"
// @Param reason query string false "TOKEN_PURCHASE or SPEND"
// @Param sort query string false "created_at direction: asc or desc (default desc)"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /internal/wallets/{user_id}/ledger [get]
func (h *WalletHandler) Ledger(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	out, err := h.svc.Ledger(c.Request.Context(), middleware.GetAppID(c), c.Param("user_id"), dto.LedgerQuery{
		Cursor:    c.Query("cursor"),
		Limit:     limit,
		EntryType: c.Query("entry_type"),
		Reason:    c.Query("reason"),
		Sort:      c.Query("sort"),
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out)
}

// ByOrder godoc
// @Summary Lookup ledger by order id
// @Tags wallet
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param order_id path string true "order id"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /internal/ledger/by-order/{order_id} [get]
func (h *WalletHandler) ByOrder(c *gin.Context) {
	out, err := h.svc.ByOrder(c.Request.Context(), middleware.GetAppID(c), c.Param("order_id"))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out)
}
