package inbound

import (
	"context"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox/platform"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
)

func New(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.Inbound, platformInterface platform.Interface) (adapter.Inbound, error) {
	if options.Type == "" {
		return nil, E.New("missing inbound type")
	}
	switch options.Type {
	case C.TypeTun:
		return NewTun(ctx, router, logger, tag, options.TunOptions, platformInterface)
	case C.TypeRedirect:
		return NewRedirect(ctx, router, logger, tag, options.RedirectOptions, platformInterface)
	case C.TypeTProxy:
		return NewTProxy(ctx, router, logger, tag, options.TProxyOptions), nil
	case C.TypeDirect:
		return NewDirect(ctx, router, logger, tag, options.DirectOptions), nil
	case C.TypeSOCKS:
		return NewSocks(ctx, router, logger, tag, options.SocksOptions), nil
	// 合并时保留此处
	case C.TypeHTTP:
		return NewHTTPPlus(ctx, router, logger, tag, options.HTTPOptions)
	case C.TypeMixed:
		return NewMixed(ctx, router, logger, tag, options.MixedOptions), nil
	case C.TypeShadowsocks:
		return NewShadowsocks(ctx, router, logger, tag, options.ShadowsocksOptions)
	// 合并时保留此处
	case C.TypeVMess:
		return NewVMessPlus(ctx, router, logger, tag, options.VMessOptions)
	case C.TypeTrojan:
		return NewTrojan(ctx, router, logger, tag, options.TrojanOptions)
	// 合并时保留此处
	case C.TypeNaive:
		return NewNaivePlus(ctx, router, logger, tag, options.NaiveOptions)
	case C.TypeHysteria:
		return NewHysteria(ctx, router, logger, tag, options.HysteriaOptions)
	case C.TypeShadowTLS:
		return NewShadowTLS(ctx, router, logger, tag, options.ShadowTLSOptions)
	// 合并时保留此处
	case C.TypeShadowTLSPlus:
		return NewShadowTLSPlus(ctx, router, logger, tag, options.ShadowTLSOptions)
	case C.TypeVLESS:
		return NewVLESS(ctx, router, logger, tag, options.VLESSOptions)
	// 合并时保留此处
	case C.TypeTUIC:
		return NewTUICPlus(ctx, router, logger, tag, options.TUICOptions)
	case C.TypeHysteria2:
		return NewHysteria2(ctx, router, logger, tag, options.Hysteria2Options)
	default:
		return nil, E.New("unknown inbound type: ", options.Type)
	}
}
