package inbound

import (
	"context"
	"net"
	"os"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/mux"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/common/uot"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/v2ray"
	"github.com/sagernet/sing-vmess/packetaddr"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	E "github.com/sagernet/sing/common/exceptions"
	F "github.com/sagernet/sing/common/format"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/ntp"
	"github.com/sagernet/sing/service"
	EC "github.com/zmaplex/sing-box-extend/edgesystem/constants"
	vmess "github.com/zmaplex/sing-box-extend/vmess"
)

var (
	_ adapter.Inbound           = (*VMessPlus)(nil)
	_ adapter.InjectableInbound = (*VMessPlus)(nil)
)

type VMessPlus struct {
	myInboundAdapter
	ctx       context.Context
	service   *vmess.Service[string]
	users     []option.VMessUser
	tlsConfig tls.ServerConfig
	transport adapter.V2RayServerTransport
}

func NewVMessPlus(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VMessInboundOptions) (*VMessPlus, error) {
	inbound := &VMessPlus{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeVMess,
			network:       []string{N.NetworkTCP},
			ctx:           ctx,
			router:        uot.NewRouter(router, logger),
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
		ctx:   ctx,
		users: options.Users,
	}
	var err error
	inbound.router, err = mux.NewRouterWithOptions(inbound.router, logger, common.PtrValueOrDefault(options.Multiplex))
	if err != nil {
		return nil, err
	}
	var serviceOptions []vmess.ServiceOption
	if timeFunc := ntp.TimeFuncFromContext(ctx); timeFunc != nil {
		serviceOptions = append(serviceOptions, vmess.ServiceWithTimeFunc(timeFunc))
	}
	if options.Transport != nil && options.Transport.Type != "" {
		serviceOptions = append(serviceOptions, vmess.ServiceWithDisableHeaderProtection())
	}

	edgeAuthenticator := service.FromContext[EC.EdgeAuthenticator](ctx)
	handle := adapter.NewUpstreamContextHandler(inbound.newConnection, inbound.newPacketConnection, inbound)
	service := vmess.NewService[string](handle, edgeAuthenticator, serviceOptions...)
	inbound.service = service
	err = service.UpdateUsers()
	if err != nil {
		return nil, err
	}

	if options.TLS != nil {
		inbound.tlsConfig, err = tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
		if err != nil {
			return nil, err
		}
	}
	if options.Transport != nil {
		inbound.transport, err = v2ray.NewServerTransport(ctx, common.PtrValueOrDefault(options.Transport), inbound.tlsConfig, (*vmessPlusTransportHandler)(inbound))
		if err != nil {
			return nil, E.Cause(err, "create server transport: ", options.Transport.Type)
		}
	}
	inbound.connHandler = inbound
	return inbound, nil
}

func (h *VMessPlus) Start() error {
	err := common.Start(
		h.service,
		h.tlsConfig,
	)
	if err != nil {
		return err
	}
	if h.transport == nil {
		return h.myInboundAdapter.Start()
	}
	if common.Contains(h.transport.Network(), N.NetworkTCP) {
		tcpListener, err := h.myInboundAdapter.ListenTCP()
		if err != nil {
			return err
		}
		go func() {
			sErr := h.transport.Serve(tcpListener)
			if sErr != nil && !E.IsClosed(sErr) {
				h.logger.Error("transport serve error: ", sErr)
			}
		}()
	}
	if common.Contains(h.transport.Network(), N.NetworkUDP) {
		udpConn, err := h.myInboundAdapter.ListenUDP()
		if err != nil {
			return err
		}
		go func() {
			sErr := h.transport.ServePacket(udpConn)
			if sErr != nil && !E.IsClosed(sErr) {
				h.logger.Error("transport serve error: ", sErr)
			}
		}()
	}
	return nil
}

func (h *VMessPlus) Close() error {
	return common.Close(
		h.service,
		&h.myInboundAdapter,
		h.tlsConfig,
		h.transport,
	)
}

func (h *VMessPlus) newTransportConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	h.injectTCP(conn, metadata)
	return nil
}

func (h *VMessPlus) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {

	var err error
	if h.tlsConfig != nil && h.transport == nil {
		conn, err = tls.ServerHandshake(ctx, conn, h.tlsConfig)
		if err != nil {
			return err
		}
	}

	return h.service.NewConnection(adapter.WithContext(log.ContextWithNewID(ctx), &metadata), conn, adapter.UpstreamMetadata(metadata))
}

func (h *VMessPlus) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	return os.ErrInvalid
}

func (h *VMessPlus) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	userUUID, loaded := auth.UserFromContext[string](ctx)
	if !loaded {
		return os.ErrInvalid
	}
	user := userUUID
	if user == "" {
		user = F.ToString(userUUID)
	} else {
		metadata.User = user
	}
	h.logger.InfoContext(ctx, "[", user, "] inbound connection to ", metadata.Destination)
	return h.router.RouteConnection(ctx, conn, metadata)
}

func (h *VMessPlus) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {

	userUUID, loaded := auth.UserFromContext[string](ctx)
	if !loaded {
		return os.ErrInvalid
	}
	user := userUUID
	if user == "" {
		user = F.ToString(userUUID)
	} else {
		metadata.User = user
	}
	if metadata.Destination.Fqdn == packetaddr.SeqPacketMagicAddress {
		metadata.Destination = M.Socksaddr{}
		conn = packetaddr.NewConn(conn.(vmess.PacketConn), metadata.Destination)
		h.logger.InfoContext(ctx, "[", user, "] inbound packet addr connection")
	} else {
		h.logger.InfoContext(ctx, "[", user, "] inbound packet connection to ", metadata.Destination)
	}
	return h.router.RoutePacketConnection(ctx, conn, metadata)
}

var _ adapter.V2RayServerTransportHandler = (*vmessTransportHandler)(nil)

type vmessPlusTransportHandler VMessPlus

func (t *vmessPlusTransportHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	return (*VMessPlus)(t).newTransportConnection(ctx, conn, adapter.InboundContext{
		Source:      metadata.Source,
		Destination: metadata.Destination,
	})
}
