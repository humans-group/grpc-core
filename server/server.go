package server

import (
	"context"
	"log"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

type Registerer interface {
	GRPCRegisterer() func(s *grpc.Server)
	HTTPRegisterer() func(ctx context.Context, mux *runtime.ServeMux, endpoint string,
		opts []grpc.DialOption) (err error)
}

type Server struct {
	services []Registerer
}

//TODO config/loader ???
func New(services ...Registerer) *Server {
	return &Server{services: services}
}

func (s *Server) Serve(ctx context.Context) {
	//TODO listen terminate OS signals and cancel context

	endpoint := "0.0.0.0:9090"

	l, err := net.Listen("tcp", endpoint)
	if err != nil {
		log.Fatal(err)
	}

	// Create a cmux.
	m := cmux.New(l)

	// Match connections in order:
	grpcL := m.Match(cmux.HTTP2())
	httpL := m.Match(cmux.HTTP1Fast())

	// grpc
	grpcS := grpc.NewServer()

	// http
	opts := []grpc.DialOption{grpc.WithInsecure()}
	mux := runtime.NewServeMux()

	for _, s := range s.services {
		s.GRPCRegisterer()(grpcS)
		if err := s.HTTPRegisterer()(ctx, mux, endpoint, opts); err != nil {
			log.Fatalf("failed to register http handler for service %T: %v", s, err)
		}
	}

	//TODO collect errors from goroutines
	go func() {
		if err := grpcS.Serve(grpcL); err != nil {
			log.Fatalf("grpc server error %v", err)
		}
	}()

	httpS := &http.Server{Handler: mux}
	go func() {
		if err := httpS.Serve(httpL); err != nil {
			//TODO not exit on ErrServerClosed, it is successful shutdown
			log.Fatalf("https server error %v", err)
		}
	}()

	// Start serving!
	go func() {
		if err := m.Serve(); err != nil {
			log.Fatalf("cmux server error: %v", err)
		}
	}()

	<-ctx.Done()
	grpcS.GracefulStop()
	//TODO shutdown streams with httpS.RegisterOnShutdown()
	if err := httpS.Shutdown(context.Background()); err != nil {
		log.Fatalf("http server shutdown error %v", err)
	}
}
