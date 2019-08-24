package consul

import (
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/resolver"
	"regexp"
	"sync"
)

const (
	defaultPort = "8500"
)

var (
	errMissingAddr  = errors.New("consul resolver: missing address")
	errAddrMisMatch = errors.New("consul resolver: invalid uri")
	regexConsul     = regexp.MustCompile("^([A-z0-9.]+)(:[0-9]{1,5})?/([A-z_]+)$")
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
		services, metainfo, err := client.Health().Service(cr.name, cr.name, true, &api.QueryOptions{WaitIndex: cr.lastIndex})
		if err != nil {
			grpclog.Errorf("error retrieving instances from Consul: %v", err)
			continue
		}
		grpclog.Infof("consul resolver got services %+v and meta %+v", services, metainfo)

		cr.lastIndex = metainfo.LastIndex
		var newAddrs []resolver.Address
		for _, service := range services {
			addr := fmt.Sprintf("%v:%v", service.Service.Address, service.Service.Port)
			newAddrs = append(newAddrs, resolver.Address{Addr: addr})
		}
		grpclog.Infof("consul resolved discovered new addresses %+v", newAddrs)
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