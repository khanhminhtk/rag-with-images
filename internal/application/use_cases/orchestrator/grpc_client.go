package orchestrator

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	pb "rag_imagetotext_texttoimage/proto"
)

func dialGRPC(ctx context.Context, host, port string) (*grpc.ClientConn, error) {
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)

	if host == "" {
		host = "localhost"
	}
	if port == "" {
		return nil, fmt.Errorf("grpc port is empty")
	}

	addr := net.JoinHostPort(host, port)

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to grpc service at %s: %w", addr, err)
	}

	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}
		if state == connectivity.Shutdown {
			conn.Close()
			return nil, fmt.Errorf("grpc connection is shutdown before ready: %s", addr)
		}
		if !conn.WaitForStateChange(dialCtx, state) {
			conn.Close()
			return nil, fmt.Errorf("timeout waiting grpc service ready at %s", addr)
		}
	}

	return conn, nil
}

func NewGRPCClient[T any](
	ctx context.Context,
	host, port string,
	newClientFunc func(grpc.ClientConnInterface) T,
) (T, *grpc.ClientConn, error) {
	var zero T
	conn, err := dialGRPC(ctx, host, port)
	if err != nil {
		return zero, nil, err
	}
	return newClientFunc(conn), conn, nil
}

func NewLLMServiceClient(
	ctx context.Context,
	host, port string,
) (pb.LlmServiceClient, *grpc.ClientConn, error) {
	return NewGRPCClient(ctx, host, port, pb.NewLlmServiceClient)
}

func NewRagServiceClient(
	ctx context.Context,
	host, port string,
) (pb.RagServiceClient, *grpc.ClientConn, error) {
	return NewGRPCClient(ctx, host, port, pb.NewRagServiceClient)
}

func NewDeepLearningServiceClient(
	ctx context.Context,
	host, port string,
) (pb.DeepLearningServiceClient, *grpc.ClientConn, error) {
	return NewGRPCClient(ctx, host, port, pb.NewDeepLearningServiceClient)
}
