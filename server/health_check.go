package server

import (
	"context"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"google.golang.org/grpc/grpclog"

	"google.golang.org/grpc/health/grpc_health_v1"
)

//health
type HealthCheck struct {
	watchServer grpc_health_v1.Health_WatchServer
}

var healthCheckResponse = &grpc_health_v1.HealthCheckResponse{
	Status: grpc_health_v1.HealthCheckResponse_SERVING,
}

// Check implements the health check interface, which directly returns to health status. There are also more complex health check strategies, such as returning based on server load.
func (h *HealthCheck) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	l := ctxzap.Extract(ctx)
	l.Info("received health check")
	return healthCheckResponse, nil
}

func (h *HealthCheck) Watch(req *grpc_health_v1.HealthCheckRequest, w grpc_health_v1.Health_WatchServer) error {
	grpclog.Infof("received health check from watching service %+v", req)
	if err := w.Send(healthCheckResponse); err != nil {
		grpclog.Errorf("failed to send health check watch respose: %v", err)
		return err
	}
	return nil
}

func (h *HealthCheck) shutdown() {
	if err := h.watchServer.Send(&grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.
		HealthCheckResponse_NOT_SERVING}); err != nil {
		grpclog.Errorf("failed to send NOT_SERVING to Health Check Watcher")
	}
}
