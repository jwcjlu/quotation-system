package server

import (
	"time"

	pb "caichip/api/bom/v1"
	"caichip/internal/conf"
	"caichip/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

// NewGRPCServer 创建 gRPC 服务
func NewGRPCServer(c *conf.Bootstrap, logger log.Logger, bomSvc *service.BomService) *grpc.Server {
	addr := ":9000"
	timeout := 30 * time.Second
	if c != nil && c.Server != nil && c.Server.Grpc != nil {
		addr = c.Server.Grpc.Addr
		if c.Server.Grpc.Timeout > 0 {
			timeout = time.Duration(c.Server.Grpc.Timeout) * time.Second
		}
	}
	srv := grpc.NewServer(
		grpc.Address(addr),
		grpc.Timeout(timeout),
	)

	pb.RegisterBomServiceServer(srv, bomSvc)

	return srv
}
