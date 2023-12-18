package inbound

import (
	"context"
	"net"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/dialer"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
	EU "github.com/zmaplex/sing-box-extend/edgesystem/users"
	shadowtls "github.com/zmaplex/sing-box-extend/shadowtls"
)

type ShadowTLSPlus struct {
	myInboundAdapter
	service *shadowtls.Service
}

func NewShadowTLSPlus(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.ShadowTLSInboundOptions) (*ShadowTLSPlus, error) {
	inbound := &ShadowTLSPlus{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeShadowTLS,
			network:       []string{N.NetworkTCP},
			ctx:           ctx,
			router:        router,
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
	}

	if options.Version == 0 {
		options.Version = 1
	}

	var handshakeForServerName map[string]shadowtls.HandshakeConfig
	if options.Version > 1 {
		handshakeForServerName = make(map[string]shadowtls.HandshakeConfig)
		for serverName, serverOptions := range options.HandshakeForServerName {
			handshakeDialer, err := dialer.New(router, serverOptions.DialerOptions)
			if err != nil {
				return nil, err
			}
			handshakeForServerName[serverName] = shadowtls.HandshakeConfig{
				Server: serverOptions.ServerOptions.Build(),
				Dialer: handshakeDialer,
			}
		}
	}
	handshakeDialer, err := dialer.New(router, options.Handshake.DialerOptions)
	if err != nil {
		return nil, err
	}
	edgeAuthenticator := service.FromContext[EU.EdgeAuthenticator](ctx)
	service, err := shadowtls.NewService(shadowtls.ServiceConfig{
		Version:  options.Version,
		Password: options.Password,
		Users: common.Map(options.Users, func(it option.ShadowTLSUser) shadowtls.User {
			return (shadowtls.User)(it)
		}),
		Handshake: shadowtls.HandshakeConfig{
			Server: options.Handshake.ServerOptions.Build(),
			Dialer: handshakeDialer,
		},
		HandshakeForServerName: handshakeForServerName,
		StrictMode:             options.StrictMode,
		Handler:                adapter.NewUpstreamContextHandler(inbound.newConnection, nil, inbound),
		Logger:                 logger,
		EdgeAuthenticator:      edgeAuthenticator,
	})
	if err != nil {
		return nil, err
	}
	inbound.service = service
	inbound.connHandler = inbound
	return inbound, nil
}

func (h *ShadowTLSPlus) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	return h.service.NewConnection(adapter.WithContext(log.ContextWithNewID(ctx), &metadata), conn, adapter.UpstreamMetadata(metadata))
}

func (h *ShadowTLSPlus) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	if userName, _ := auth.UserFromContext[string](ctx); userName != "" {
		metadata.User = userName
		h.logger.InfoContext(ctx, "[", userName, "] inbound connection to ", metadata.Destination)
	} else {
		h.logger.InfoContext(ctx, "inbound connection to ", metadata.Destination)
	}
	return h.router.RouteConnection(ctx, conn, metadata)
}
