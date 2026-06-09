package rpc

import (
	"fmt"

	graphapp "github.com/tinoosan/agen8/internal/services/graph/app"
	"github.com/tinoosan/agen8/internal/services/graph/contextlink"
	graphrpc "github.com/tinoosan/agen8/internal/services/graph/rpc"
)

const (
	MethodGraphNode          = "graph.node"
	MethodGraphSearch        = "graph.search"
	MethodGraphLink          = "graph.link"
	MethodGraphUnlink        = "graph.unlink"
	MethodGraphLinksBySource = "graph.linksBySource"
	MethodGraphLinksByTarget = "graph.linksByTarget"
)

func RegisterGraph(reg *Registry, graphSvc *graphapp.Service, links contextlink.Reader) error {
	handler, err := graphrpc.NewHandler(graphSvc, links)
	if err != nil {
		return err
	}
	if reg == nil {
		return fmt.Errorf("rpc registry is required")
	}
	return RegisterHandlers(
		func() error { return AddBoundHandler(reg, MethodGraphNode, false, handler.Node) },
		func() error { return AddBoundHandler(reg, MethodGraphSearch, false, handler.Search) },
		func() error { return AddBoundHandler(reg, MethodGraphLink, false, handler.Link) },
		func() error { return AddBoundHandler(reg, MethodGraphUnlink, false, handler.Unlink) },
		func() error { return AddBoundHandler(reg, MethodGraphLinksBySource, false, handler.LinksBySource) },
		func() error { return AddBoundHandler(reg, MethodGraphLinksByTarget, false, handler.LinksByTarget) },
	)
}
