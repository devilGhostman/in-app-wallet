package handler

import (
	"wallet-svc/internal/dto"
	"wallet-svc/internal/metrics"
	"wallet-svc/internal/middleware"
	"wallet-svc/internal/services"
	"wallet-svc/pkg/response"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	svc      *services.WalletService
	counters *metrics.Counters
}

func NewAdminHandler(svc *services.WalletService, counters *metrics.Counters) *AdminHandler {
	return &AdminHandler{svc: svc, counters: counters}
}

// Freeze godoc
// @Summary Freeze a wallet
// @Tags admin
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param user_id path string true "user id"
// @Param body body dto.FreezeRequest true "reason"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /admin/wallets/{user_id}/freeze [post]
func (h *AdminHandler) Freeze(c *gin.Context) {
	h.changeStatus(c, "freeze")
}

// Unfreeze godoc
// @Summary Unfreeze a wallet
// @Tags admin
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param user_id path string true "user id"
// @Param body body dto.FreezeRequest true "reason"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /admin/wallets/{user_id}/unfreeze [post]
func (h *AdminHandler) Unfreeze(c *gin.Context) {
	h.changeStatus(c, "unfreeze")
}

// Close godoc
// @Summary Close a wallet
// @Tags admin
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-App-Id header string true "Application id"
// @Param X-Request-ID header string false "Request correlation id"
// @Param user_id path string true "user id"
// @Param body body dto.FreezeRequest true "reason"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /admin/wallets/{user_id}/close [post]
func (h *AdminHandler) Close(c *gin.Context) {
	h.changeStatus(c, "close")
}

func (h *AdminHandler) changeStatus(c *gin.Context, action string) {
	var req dto.FreezeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.BadRequest(response.CodeInvalidBody, "Invalid request body", err.Error()))
		return
	}
	requestID := middleware.GetRequestID(c)
	appID := middleware.GetAppID(c)
	userID := c.Param("user_id")
	var (
		out dto.WalletResponse
		err error
	)
	switch action {
	case "freeze":
		out, err = h.svc.Freeze(c.Request.Context(), appID, userID, req.Reason, requestID)
	case "unfreeze":
		out, err = h.svc.Unfreeze(c.Request.Context(), appID, userID, req.Reason, requestID)
	case "close":
		out, err = h.svc.Close(c.Request.Context(), appID, userID, req.Reason, requestID)
	default:
		response.Error(c, response.BadRequest(response.CodeInvalidRequest, "Unknown status action"))
		return
	}
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out)
}

// Reconcile godoc
// @Summary Run balance invariant check
// @Tags admin
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-Request-ID header string false "Request correlation id"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /admin/reconciliation [get]
func (h *AdminHandler) Reconcile(c *gin.Context) {
	out, err := h.svc.Reconcile(c.Request.Context())
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.OK(c, out)
}

// Metrics godoc
// @Summary In-process counters
// @Tags admin
// @Produce json
// @Param Authorization header string true "Bearer HS256 JWT signed with JWT_SECRET"
// @Param X-Scopes header string true "Comma/space-separated scopes (e.g. wallet:admin)"
// @Param X-Request-ID header string false "Request correlation id"
// @Success 200 {object} response.SuccessEnvelope
// @Failure 401 {object} response.ErrorEnvelope
// @Failure 403 {object} response.ErrorEnvelope
// @Security BearerAuth
// @Router /admin/metrics [get]
func (h *AdminHandler) Metrics(c *gin.Context) {
	response.OK(c, h.counters.Snapshot())
}
