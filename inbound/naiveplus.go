package inbound

import (
	"context"
	"net"
	"net/http"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/common/uot"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	sHttp "github.com/sagernet/sing/protocol/http"
	"github.com/sagernet/sing/service"
	EC "github.com/zmaplex/sing-box-extend/edgesystem/constants"
)

var _ adapter.Inbound = (*NaivePlus)(nil)

type NaivePlus struct {
	myInboundAdapter
	tlsConfig  tls.ServerConfig
	httpServer *http.Server
	h3Server   any
}

func NewNaivePlus(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.NaiveInboundOptions) (*NaivePlus, error) {
	inbound := &NaivePlus{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeNaive,
			network:       options.Network.Build(),
			ctx:           ctx,
			router:        uot.NewRouter(router, logger),
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
	}
	if common.Contains(inbound.network, N.NetworkUDP) {
		if options.TLS == nil || !options.TLS.Enabled {
			return nil, E.New("TLS is required for QUIC server")
		}
	}
	if len(options.Users) == 0 {
		return nil, E.New("missing users")
	}
	if options.TLS != nil {
		tlsConfig, err := tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
		if err != nil {
			return nil, err
		}
		inbound.tlsConfig = tlsConfig
	}
	return inbound, nil
}

func (n *NaivePlus) Start() error {
	var tlsConfig *tls.STDConfig
	if n.tlsConfig != nil {
		err := n.tlsConfig.Start()
		if err != nil {
			return E.Cause(err, "create TLS config")
		}
		tlsConfig, err = n.tlsConfig.Config()
		if err != nil {
			return err
		}
	}

	if common.Contains(n.network, N.NetworkTCP) {
		tcpListener, err := n.ListenTCP()
		if err != nil {
			return err
		}
		n.httpServer = &http.Server{
			Handler:   n,
			TLSConfig: tlsConfig,
			BaseContext: func(listener net.Listener) context.Context {
				return n.ctx
			},
		}
		go func() {
			var sErr error
			if tlsConfig != nil {
				sErr = n.httpServer.ServeTLS(tcpListener, "", "")
			} else {
				sErr = n.httpServer.Serve(tcpListener)
			}
			if sErr != nil && !E.IsClosedOrCanceled(sErr) {
				n.logger.Error("http server serve error: ", sErr)
			}
		}()
	}

	if common.Contains(n.network, N.NetworkUDP) {
		err := n.configureHTTP3Listener()
		if !C.WithQUIC && len(n.network) > 1 {
			n.logger.Warn(E.Cause(err, "naive http3 disabled"))
		} else if err != nil {
			return err
		}
	}

	return nil
}

func (n *NaivePlus) Close() error {
	return common.Close(
		&n.myInboundAdapter,
		common.PtrOrNil(n.httpServer),
		n.h3Server,
		n.tlsConfig,
	)
}

func (n *NaivePlus) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	ctx := log.ContextWithNewID(request.Context())
	if request.Method != "CONNECT" {
		rejectHTTP(writer, http.StatusBadRequest)
		n.badRequest(ctx, request, E.New("not CONNECT request"))
		return
	} else if request.Header.Get("Padding") == "" {
		rejectHTTP(writer, http.StatusBadRequest)
		n.badRequest(ctx, request, E.New("missing naive padding"))
		return
	}
	edgeAuthenticator := service.FromContext[EC.EdgeAuthenticator](ctx)

	userName, password, authOk := sHttp.ParseBasicAuth(request.Header.Get("Proxy-Authorization"))

	authOk = false
	for uname, user := range edgeAuthenticator.Users() {
		if uname == userName && user.Password == password {
			authOk = true
			break
		}
	}

	if !authOk {
		rejectHTTP(writer, http.StatusProxyAuthRequired)
		n.badRequest(ctx, request, E.New("authorization failed"))
		return
	}
	writer.Header().Set("Padding", generateNaivePaddingHeader())
	writer.WriteHeader(http.StatusOK)
	writer.(http.Flusher).Flush()

	hostPort := request.URL.Host
	if hostPort == "" {
		hostPort = request.Host
	}
	source := sHttp.SourceAddress(request)
	destination := M.ParseSocksaddr(hostPort)

	if hijacker, isHijacker := writer.(http.Hijacker); isHijacker {
		conn, _, err := hijacker.Hijack()
		if err != nil {
			n.badRequest(ctx, request, E.New("hijack failed"))
			return
		}
		n.newConnection(ctx, &naiveH1Conn{Conn: conn}, userName, source, destination)
	} else {
		n.newConnection(ctx, &naiveH2Conn{reader: request.Body, writer: writer, flusher: writer.(http.Flusher)}, userName, source, destination)
	}
}

func (n *NaivePlus) newConnection(ctx context.Context, conn net.Conn, userName string, source, destination M.Socksaddr) {
	if userName != "" {
		n.logger.InfoContext(ctx, "[", userName, "] inbound connection from ", source)
		n.logger.InfoContext(ctx, "[", userName, "] inbound connection to ", destination)
	} else {
		n.logger.InfoContext(ctx, "inbound connection from ", source)
		n.logger.InfoContext(ctx, "inbound connection to ", destination)
	}
	hErr := n.router.RouteConnection(ctx, conn, n.createMetadata(conn, adapter.InboundContext{
		Source:      source,
		Destination: destination,
		User:        userName,
	}))
	if hErr != nil {
		conn.Close()
		n.NewError(ctx, E.Cause(hErr, "process connection from ", source))
	}
}

func (n *NaivePlus) badRequest(ctx context.Context, request *http.Request, err error) {
	n.NewError(ctx, E.Cause(err, "process connection from ", request.RemoteAddr))
}
