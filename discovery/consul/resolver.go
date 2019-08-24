package consul

import (
	"fmt"
	"sync"

	"github.com/hashicorp/consul/api"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/resolver"
)

func Init() {
	resolver.Register(NewBuilder())
}

type consulBuilder struct {
}

type consulResolver struct {
	address              string
	wg                   sync.WaitGroup
	cc                   resolver.ClientConn
	name                 string
	disableServiceConfig bool
	lastIndex            uint64
}

func NewBuilder() resolver.Builder {
	return &consulBuilder{}
}

func (cb *consulBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOption) (resolver.Resolver, error) {
	cr := &consulResolver{
		address:              target.Authority,
		name:                 target.Endpoint,
		cc:                   cc,
		disableServiceConfig: opts.DisableServiceConfig,
		lastIndex:            0,
	}

	cr.wg.Add(1)
	go cr.watcher()
	return cr, nil
}

func (cr *consulResolver) watcher() {
	config := api.DefaultConfig()
	config.Address = cr.address
	client, err := api.NewClient(config)
	if err != nil {
		grpclog.Errorf("error create consul client: %v", err)
		return
	}

	for {
		services, metainfo, err := client.Health().Service(cr.name, env, true,
			&api.QueryOptions{WaitIndex: cr.lastIndex})
		if err != nil {
			grpclog.Errorf("error retrieving instances from Consul: %v", err)
			continue
		}

		cr.lastIndex = metainfo.LastIndex
		var newAddrs []resolver.Address
		for _, service := range services {
			addr := fmt.Sprintf("%v:%v", service.Service.Address, service.Service.Port)
			newAddrs = append(newAddrs, resolver.Address{Addr: addr})
		}

		grpclog.Infof("consul resolver got new addresses %+v", newAddrs)

		cr.cc.UpdateState(resolver.State{
			Addresses: newAddrs,
		})
		//cr.cc.NewServiceConfig(cr.name)
	}

}

func (cb *consulBuilder) Scheme() string {
	return "consul"
}

func (cr *consulResolver) ResolveNow(opt resolver.ResolveNowOption) {
}

func (cr *consulResolver) Close() {
}
