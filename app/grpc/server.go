package grpc

import (
	"context"
	"errors"

	"github.com/vibast-solutions/ms-go-payments/app/mapper"
	"github.com/vibast-solutions/ms-go-payments/app/service"
	"github.com/vibast-solutions/ms-go-payments/app/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	types.UnimplementedPaymentsServiceServer
	paymentService *service.PaymentService
}

func NewServer(paymentService *service.PaymentService) *Server {
	return &Server{paymentService: paymentService}
}

func (s *Server) Health(_ context.Context, _ *types.HealthRequest) (*types.HealthResponse, error) {
	return &types.HealthResponse{Status: "ok"}, nil
}

func (s *Server) CreatePayment(ctx context.Context, req *types.CreatePaymentRequest) (*types.PaymentEnvelopeResponse, error) {
	l := loggerWithContext(ctx)
	if err := req.Validate(); err != nil {
		l.WithError(err).Debug("Create payment validation failed")
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	item, err := s.paymentService.CreatePayment(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRequest), errors.Is(err, service.ErrInvalidStatus), errors.Is(err, service.ErrProviderUnsupported):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, service.ErrPaymentAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, err.Error())
		default:
			l.WithError(err).Error("Create payment failed")
			return nil, status.Error(codes.Internal, "internal server error")
		}
	}

	return &types.PaymentEnvelopeResponse{Payment: mapper.PaymentToProto(item)}, nil
}

func (s *Server) GetPayment(ctx context.Context, req *types.GetPaymentRequest) (*types.PaymentEnvelopeResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	item, err := s.paymentService.GetPayment(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, service.ErrPaymentNotFound) {
			return nil, status.Error(codes.NotFound, "payment not found")
		}
		return nil, status.Error(codes.Internal, "internal server error")
	}

	return &types.PaymentEnvelopeResponse{Payment: mapper.PaymentToProto(item)}, nil
}

func (s *Server) ListPayments(ctx context.Context, req *types.ListPaymentsRequest) (*types.ListPaymentsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	items, err := s.paymentService.ListPayments(ctx, req)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal server error")
	}

	return &types.ListPaymentsResponse{Payments: mapper.PaymentsToProto(items)}, nil
}

func (s *Server) CancelPayment(ctx context.Context, req *types.CancelPaymentRequest) (*types.PaymentEnvelopeResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	item, err := s.paymentService.CancelPayment(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentNotFound):
			return nil, status.Error(codes.NotFound, "payment not found")
		case errors.Is(err, service.ErrInvalidStatus):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		default:
			return nil, status.Error(codes.Internal, "internal server error")
		}
	}

	return &types.PaymentEnvelopeResponse{Payment: mapper.PaymentToProto(item)}, nil
}

func (s *Server) HandleProviderCallback(ctx context.Context, req *types.HandleProviderCallbackRequest) (*types.MessageResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	_, err := s.paymentService.HandleProviderCallback(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProviderUnsupported), errors.Is(err, service.ErrCallbackRejected), errors.Is(err, service.ErrInvalidRequest):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, service.ErrPaymentNotFound):
			return nil, status.Error(codes.NotFound, "payment not found")
		default:
			return nil, status.Error(codes.Internal, "internal server error")
		}
	}

	return &types.MessageResponse{Message: "Provider callback processed"}, nil
}
