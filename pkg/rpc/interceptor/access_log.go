package interceptor

import (
	"context"
	"github.com/piupuer/go-helper/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"time"
)

func AccessLog(options ...func(*AccessLogOptions)) grpc.UnaryServerInterceptor {
	ops := getAccessLogOptionsOrSetDefault(nil)
	for _, f := range options {
		f(ops)
	}
	return func(ctx context.Context, r interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		startTime := time.Now()
		ops.logger.Info(
			ctx,
			"req: %s",
			utils.Struct2Json(r),
		)

		rp, err := handler(ctx, r)

		endTime := time.Now()
		// calc request exec time
		execTime := endTime.Sub(startTime).String()

		fullMethod := info.FullMethod
		addr := ""
		if p, ok := peer.FromContext(ctx); ok {
			addr = p.Addr.String()
		}
		code := status.Code(err).String()
		if err != nil {
			ops.logger.Error(
				ctx,
				"%s %s %s RPC code: '%s', RPC err: '%v'",
				fullMethod,
				execTime,
				addr,
				code,
				err,
			)
		} else {
			if ops.detail {
				ops.logger.Info(
					ctx,
					"RPC code: '%s', resp: %s",
					code,
					utils.Struct2Json(rp),
				)
			} else {
				ops.logger.Info(
					ctx,
					"RPC code: '%s'",
					code,
				)
			}
		}
		return rp, err
	}
}
