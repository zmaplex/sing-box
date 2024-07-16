package inbound

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-quic/hysteria"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
	EC "github.com/zmaplex/sing-box-extend/edgesystem/constants"
	"github.com/zmaplex/sing-box-extend/inbound/hysteria2"
)

var _ adapter.Inbound = (*Hysteria2Plus)(nil)

type Hysteria2Plus struct {
	myInboundAdapter
	tlsConfig tls.ServerConfig
	service   *hysteria2.Service
}

func NewHysteria2Plus(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.Hysteria2InboundOptions) (*Hysteria2Plus, error) {
	options.UDPFragmentDefault = true
	if options.TLS == nil || !options.TLS.Enabled {
		return nil, C.ErrTLSRequired
	}
	tlsConfig, err := tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
	if err != nil {
		return nil, err
	}
	var salamanderPassword string
	if options.Obfs != nil {
		if options.Obfs.Password == "" {
			return nil, E.New("missing obfs password")
		}
		switch options.Obfs.Type {
		case hysteria2.ObfsTypeSalamander:
			salamanderPassword = options.Obfs.Password
		default:
			return nil, E.New("unknown obfs type: ", options.Obfs.Type)
		}
	}
	var masqueradeHandler http.Handler
	if options.Masquerade != "" {
		masqueradeURL, err := url.Parse(options.Masquerade)
		if err != nil {
			return nil, E.Cause(err, "parse masquerade URL")
		}
		switch masqueradeURL.Scheme {
		case "file":
			masqueradeHandler = http.FileServer(http.Dir(masqueradeURL.Path))
		case "http", "https":
			masqueradeHandler = &httputil.ReverseProxy{
				Rewrite: func(r *httputil.ProxyRequest) {
					r.SetURL(masqueradeURL)
					r.Out.Host = r.In.Host
				},
				ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
					w.WriteHeader(http.StatusBadGateway)
				},
			}
		default:
			return nil, E.New("unknown masquerade URL scheme: ", masqueradeURL.Scheme)
		}
	}
	inbound := &Hysteria2Plus{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeHysteria2,
			network:       []string{N.NetworkUDP},
			ctx:           ctx,
			router:        router,
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
		tlsConfig: tlsConfig,
	}
	var udpTimeout time.Duration
	if options.UDPTimeout != 0 {
		udpTimeout = time.Duration(options.UDPTimeout)
	} else {
		udpTimeout = C.UDPTimeout
	}
	edgeAuthenticator := service.FromContext[EC.EdgeAuthenticator](ctx)

	service, err := hysteria2.NewService(edgeAuthenticator, hysteria2.ServiceOptions{
		Context:               ctx,
		Logger:                logger,
		BrutalDebug:           options.BrutalDebug,
		SendBPS:               uint64(options.UpMbps * hysteria.MbpsToBps),
		ReceiveBPS:            uint64(options.DownMbps * hysteria.MbpsToBps),
		SalamanderPassword:    salamanderPassword,
		TLSConfig:             tlsConfig,
		IgnoreClientBandwidth: options.IgnoreClientBandwidth,
		UDPTimeout:            udpTimeout,
		Handler:               adapter.NewUpstreamHandler(adapter.InboundContext{}, inbound.newConnection, inbound.newPacketConnection, nil),
		MasqueradeHandler:     masqueradeHandler,
	})
	if err != nil {
		return nil, err
	}
	service.UpdateUsers()
	inbound.service = service
	return inbound, nil
}

func (h *Hysteria2Plus) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	ctx = log.ContextWithNewID(ctx)
	metadata = h.createMetadata(conn, metadata)
	h.logger.InfoContext(ctx, "inbound connection from ", metadata.Source)
	username, _ := auth.UserFromContext[string](ctx)
	if username != "" {
		metadata.User = username
		h.logger.InfoContext(ctx, "[", username, "] inbound connection to ", metadata.Destination)
	} else {
		h.logger.InfoContext(ctx, "inbound connection to ", metadata.Destination)
	}

	return h.router.RouteConnection(ctx, conn, metadata)
}

func (h *Hysteria2Plus) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	ctx = log.ContextWithNewID(ctx)
	metadata = h.createPacketMetadata(conn, metadata)
	h.logger.InfoContext(ctx, "inbound packet connection from ", metadata.Source)
	username, _ := auth.UserFromContext[string](ctx)
	if username != "" {
		metadata.User = username
		h.logger.InfoContext(ctx, "[", username, "] inbound packet connection to ", metadata.Destination)
	} else {
		h.logger.InfoContext(ctx, "inbound packet connection to ", metadata.Destination)
	}

	return h.router.RoutePacketConnection(ctx, conn, metadata)
}

func (h *Hysteria2Plus) Start() error {
	if h.tlsConfig != nil {
		err := h.tlsConfig.Start()
		if err != nil {
			return err
		}
	}
	packetConn, err := h.myInboundAdapter.ListenUDP()
	if err != nil {
		return err
	}
	return h.service.Start(packetConn)
}

func (h *Hysteria2Plus) Close() error {
	return common.Close(
		&h.myInboundAdapter,
		h.tlsConfig,
		common.PtrOrNil(h.service),
	)
}
