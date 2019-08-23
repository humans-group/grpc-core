package consul

import (
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/consul/api"
	"google.golang.org/grpc/grpclog"
)

type Service struct {
	IP   string
	Port int
	Tag  []string
	Name string
}

// populate with ld flags
var (
	env     string
	ip      string
	port    string
	portInt int
)

func init() {
	var err error
	portInt, err = strconv.Atoi(port)
	if err != nil {
		panic(fmt.Sprintf("failed to convert port %s string to int: %v", port, err))
	}
}

func RegisterService(cfg Config) {
	grpclog.Infof("consul envs %v %v %v", env, ip, port)

	consulConfig := api.DefaultConfig()
	consulConfig.Address = cfg.Endpoint
	client, err := api.NewClient(consulConfig)
	if err != nil {
		grpclog.Fatalf("NewClient error: %v", err)
		return
	}
	agent := client.Agent()
	interval := 10 * time.Second
	deregister := time.Minute

	reg := &api.AgentServiceRegistration{
		ID:      fmt.Sprintf("%v-%v-%v", cfg.Name, ip, port),
		Name:    cfg.Name,
		Tags:    []string{env},
		Port:    portInt,
		Address: ip,
		Check: &api.AgentServiceCheck{
			// health check interval
			Interval: interval.String(),
			// grpc support, address to perform health check, service will be passed to HealthCheck function
			GRPC: fmt.Sprintf("%v:%v/%v", ip, port, cfg.Name),
			// logout time, equivalent to expiration time
			DeregisterCriticalServiceAfter: deregister.String(),
		},
	}

	grpclog.Infof("registering to %v", cfg.Endpoint)
	if err := agent.ServiceRegister(reg); err != nil {
		grpclog.Fatalf("Service Register error: %v", err)
		return
	}
}
