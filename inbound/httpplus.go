package inbound

import (
	std_bufio "bufio"
	"context"
	"net"
	"os"

	"github.com/sagernet/sing/service"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/common/uot"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"
	EC "github.com/zmaplex/sing-box-extend/edgesystem/constants"
	"github.com/zmaplex/sing-box-extend/inbound/http"
)

var (
	_ adapter.Inbound           = (*HTTP)(nil)
	_ adapter.InjectableInbound = (*HTTP)(nil)
)

type HTTPPlus struct {
	myInboundAdapter
	authenticator EC.EdgeAuthenticator
	tlsConfig     tls.ServerConfig
}

func NewHTTPPlus(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.HTTPMixedInboundOptions) (*HTTPPlus, error) {
	inbound := &HTTPPlus{
		myInboundAdapter: myInboundAdapter{
			protocol:       C.TypeHTTP,
			network:        []string{N.NetworkTCP},
			ctx:            ctx,
			router:         uot.NewRouter(router, logger),
			logger:         logger,
			tag:            tag,
			listenOptions:  options.ListenOptions,
			setSystemProxy: options.SetSystemProxy,
		},
		authenticator: service.FromContext[EC.EdgeAuthenticator](ctx),
	}
	if options.TLS != nil {
		tlsConfig, err := tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
		if err != nil {
			return nil, err
		}
		inbound.tlsConfig = tlsConfig
	}
	inbound.connHandler = inbound
	return inbound, nil
}

func (h *HTTPPlus) Start() error {
	if h.tlsConfig != nil {
		err := h.tlsConfig.Start()
		if err != nil {
			return E.Cause(err, "create TLS config")
		}
	}
	return h.myInboundAdapter.Start()
}

func (h *HTTPPlus) Close() error {
	return common.Close(
		&h.myInboundAdapter,
		h.tlsConfig,
	)
}

func (h *HTTPPlus) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	var err error
	if h.tlsConfig != nil {
		conn, err = tls.ServerHandshake(ctx, conn, h.tlsConfig)
		if err != nil {
			return err
		}
	}
	return http.HandleConnection(ctx, conn, std_bufio.NewReader(conn), h.authenticator, h.upstreamUserHandler(metadata), adapter.UpstreamMetadata(metadata))
}

func (h *HTTPPlus) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	return os.ErrInvalid
}
