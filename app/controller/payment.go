package controller

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/vibast-solutions/ms-go-payments/app/factory"
	"github.com/vibast-solutions/ms-go-payments/app/mapper"
	"github.com/vibast-solutions/ms-go-payments/app/service"
	"github.com/vibast-solutions/ms-go-payments/app/types"
)

type PaymentController struct {
	paymentService *service.PaymentService
	logger         logrus.FieldLogger
}

func NewPaymentController(paymentService *service.PaymentService) *PaymentController {
	return &PaymentController{
		paymentService: paymentService,
		logger:         factory.NewModuleLogger("payments-controller"),
	}
}

func (c *PaymentController) Health(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, &types.HealthResponse{Status: "ok"})
}

func (c *PaymentController) CreatePayment(ctx echo.Context) error {
	req, err := types.NewCreatePaymentRequestFromContext(ctx)
	if err != nil {
		return c.writeError(ctx, http.StatusBadRequest, "invalid request body")
	}
	if err := req.Validate(); err != nil {
		return c.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	item, err := c.paymentService.CreatePayment(ctx.Request().Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRequest), errors.Is(err, service.ErrInvalidStatus), errors.Is(err, service.ErrProviderUnsupported):
			return c.writeError(ctx, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrPaymentAlreadyExists):
			return c.writeError(ctx, http.StatusConflict, err.Error())
		default:
			c.logger.WithError(err).Error("Create payment failed")
			return c.writeError(ctx, http.StatusInternalServerError, "internal server error")
		}
	}

	return ctx.JSON(http.StatusCreated, &types.PaymentEnvelopeResponse{Payment: mapper.PaymentToProto(item)})
}

func (c *PaymentController) GetPayment(ctx echo.Context) error {
	req, err := types.NewGetPaymentRequestFromContext(ctx)
	if err != nil {
		return c.writeError(ctx, http.StatusBadRequest, "invalid request")
	}
	if err := req.Validate(); err != nil {
		return c.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	item, err := c.paymentService.GetPayment(ctx.Request().Context(), req.GetId())
	if err != nil {
		if errors.Is(err, service.ErrPaymentNotFound) {
			return c.writeError(ctx, http.StatusNotFound, "payment not found")
		}
		c.logger.WithError(err).Error("Get payment failed")
		return c.writeError(ctx, http.StatusInternalServerError, "internal server error")
	}

	return ctx.JSON(http.StatusOK, &types.PaymentEnvelopeResponse{Payment: mapper.PaymentToProto(item)})
}

func (c *PaymentController) ListPayments(ctx echo.Context) error {
	req, err := types.NewListPaymentsRequestFromContext(ctx)
	if err != nil {
		return c.writeError(ctx, http.StatusBadRequest, "invalid request")
	}
	if err := req.Validate(); err != nil {
		return c.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	items, err := c.paymentService.ListPayments(ctx.Request().Context(), req)
	if err != nil {
		c.logger.WithError(err).Error("List payments failed")
		return c.writeError(ctx, http.StatusInternalServerError, "internal server error")
	}

	return ctx.JSON(http.StatusOK, &types.ListPaymentsResponse{Payments: mapper.PaymentsToProto(items)})
}

func (c *PaymentController) CancelPayment(ctx echo.Context) error {
	req, err := types.NewCancelPaymentRequestFromContext(ctx)
	if err != nil {
		return c.writeError(ctx, http.StatusBadRequest, "invalid request")
	}
	if err := req.Validate(); err != nil {
		return c.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	item, err := c.paymentService.CancelPayment(ctx.Request().Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentNotFound):
			return c.writeError(ctx, http.StatusNotFound, "payment not found")
		case errors.Is(err, service.ErrInvalidStatus):
			return c.writeError(ctx, http.StatusBadRequest, err.Error())
		default:
			c.logger.WithError(err).Error("Cancel payment failed")
			return c.writeError(ctx, http.StatusInternalServerError, "internal server error")
		}
	}

	return ctx.JSON(http.StatusOK, &types.PaymentEnvelopeResponse{Payment: mapper.PaymentToProto(item)})
}

func (c *PaymentController) HandleProviderCallback(ctx echo.Context) error {
	req, err := types.NewHandleProviderCallbackRequestFromContext(ctx)
	if err != nil {
		return c.writeError(ctx, http.StatusBadRequest, "invalid request body")
	}
	if err := req.Validate(); err != nil {
		return c.writeError(ctx, http.StatusBadRequest, err.Error())
	}

	_, err = c.paymentService.HandleProviderCallback(ctx.Request().Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProviderUnsupported), errors.Is(err, service.ErrCallbackRejected), errors.Is(err, service.ErrInvalidRequest):
			return c.writeError(ctx, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrPaymentNotFound):
			return c.writeError(ctx, http.StatusNotFound, "payment not found")
		default:
			c.logger.WithError(err).Error("Handle provider callback failed")
			return c.writeError(ctx, http.StatusInternalServerError, "internal server error")
		}
	}

	return ctx.JSON(http.StatusOK, &types.MessageResponse{Message: "Provider callback processed"})
}

func (c *PaymentController) writeError(ctx echo.Context, statusCode int, message string) error {
	return ctx.JSON(statusCode, &types.ErrorResponse{Error: message})
}
