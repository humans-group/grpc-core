package server

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	grpc_opentracing "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/mkorolyov/core/config"
	"github.com/mkorolyov/core/logger"
	"github.com/mkorolyov/core/tracer"
	"github.com/soheilhy/cmux"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type Registerer interface {
	GRPCRegisterer() func(s *grpc.Server)
	HTTPRegisterer() func(ctx context.Context, mux *runtime.ServeMux, endpoint string,
		opts []grpc.DialOption) (err error)
}

type Server struct {
	services []Registerer
	cfg      Config
	grpcProxyMux *runtime.ServeMux
	exitFunc func(code int)
	ctx      context.Context
	log      *zap.Logger
}

func New(loader config.Loader, services ...Registerer) *Server {
	s := &Server{
		services: services,
		log:      logger.Init(loader),
	}

	loader.MustLoad("Server", &s.cfg)

	s.AddExitFunc(func(_ int) {
		if err := s.log.Sync(); err != nil {
			panic(fmt.Sprintf("failed to flush logger before exit %v", err))
		}
	})

	tracerCloser, err := tracer.InitJaeger(s.cfg.Name, loader, s.log)
	if err != nil {
		s.log.Sugar().Errorf("failed to proper init jaeger tracing %v", err)
	}
	if tracerCloser != nil {
		s.AddExitFunc(func(_ int) {
			if err := tracerCloser.Close(); err != nil {
				s.log.Sugar().Errorf("failed to close jaeger client: %v", err)
			}
		})
	}

	return s
}

func (s *Server) Serve(ctx context.Context) {
	var cancelFunc func()
	s.ctx, cancelFunc = context.WithCancel(ctx)
	go s.watchShutdown(cancelFunc)

	l, err := net.Listen("tcp", s.cfg.Endpoint)
	if err != nil {
		s.log.Sugar().Panicf("failed to connect to %s: %v", s.cfg.Endpoint, err)
	}

	connMultiplexer := cmux.New(l)
	grpcL := connMultiplexer.Match(cmux.HTTP2())
	httpL := connMultiplexer.Match(cmux.HTTP1Fast())

	grpc_zap.ReplaceGrpcLogger(s.log)

	// grpc
	grpcS := grpc.NewServer(
		grpc_middleware.WithUnaryServerChain(
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.UnaryServerInterceptor(s.log, []grpc_zap.Option{
				grpc_zap.WithLevels(codeToLevel),
			}...),
			grpc_zap.PayloadUnaryServerInterceptor(s.log, s.payloadLoggingDecider),
			grpc_opentracing.UnaryServerInterceptor(),
			grpc_prometheus.UnaryServerInterceptor,
		),
	)

	grpc_prometheus.EnableHandlingTimeHistogram()

	s.grpcProxyMux = runtime.NewServeMux()
	for _, se := range s.services {
		se.GRPCRegisterer()(grpcS)
		if err := se.HTTPRegisterer()(s.ctx, s.grpcProxyMux, s.cfg.Endpoint, []grpc.DialOption{grpc.WithInsecure()}); err != nil {
			s.log.Sugar().Panicf("failed to register http handler for service %T: %v", s, err)
		}
	}

	//TODO collect errors from goroutines
	go func() {
		if err := grpcS.Serve(grpcL); err != nil {
			s.log.Sugar().Panicf("grpc server error %v", err)
		}
	}()

	httpS := &http.Server{Handler: s}
	go func() {
		if err := httpS.Serve(httpL); err != nil {
			//TODO not exit on ErrServerClosed, it is successful watchShutdown
			s.log.Sugar().Errorf("https server error %v", err)
		}
	}()

	go func() {
		if err := connMultiplexer.Serve(); err != nil {
			s.log.Sugar().Errorf("cmux server error: %v", err)
		}
	}()

	<-s.ctx.Done()
	grpcS.GracefulStop()
	//TODO watchShutdown streams with httpS.RegisterOnShutdown()
	if err := httpS.Shutdown(s.ctx); err != nil {
		s.log.Sugar().Errorf("http server watchShutdown error %v", err)
	}

	s.log.Sugar().Info("microservice gracefully stopped")
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/metrics") {
		promhttp.Handler().ServeHTTP(w, r)
		return
	}

	s.grpcProxyMux.ServeHTTP(w, r)
}

func (s *Server) AddExitFunc(fn func(code int)) {
	exitFunc := s.exitFunc
	if exitFunc == nil {
		exitFunc = os.Exit
	}

	s.exitFunc = func(code int) {
		fn(code)
		exitFunc(code)
	}
}

func (s *Server) exit(code int) {
	if s.exitFunc == nil {
		os.Exit(code)
	} else {
		s.exitFunc(code)
	}
}

func (s *Server) watchShutdown(cancelFunc context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
	sig := <-sigChan
	log.Printf("received %s signal from OS\n", sig.String())
	cancelFunc()
}

func codeToLevel(code codes.Code) zapcore.Level {
	switch code {
	case codes.OK:
		return zapcore.InfoLevel
	case codes.Unauthenticated, codes.PermissionDenied:
		return zapcore.WarnLevel
	default:
		return zapcore.ErrorLevel
	}
}

func (s *Server) payloadLoggingDecider(ctx context.Context, fullMethodName string, servingObject interface{}) bool {
	return *s.cfg.LogPayloads
}
