package grpcapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/masterchef/masterchef/internal/state"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

type jsonCodec struct{}

func (jsonCodec) Name() string {
	return "json"
}

func (jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

type HealthRequest struct{}

type HealthResponse struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type ListRunsRequest struct {
	Limit int `json:"limit,omitempty"`
}

type ListRunsResponse struct {
	Count int               `json:"count"`
	Items []state.RunRecord `json:"items"`
}

type Service struct {
	baseDir string
}

type ControlServer interface {
	Health(context.Context, *HealthRequest) (*HealthResponse, error)
	ListRuns(context.Context, *ListRunsRequest) (*ListRunsResponse, error)
}

func New(baseDir string) *Service {
	return &Service{baseDir: baseDir}
}

func (s *Service) Register(grpcServer *grpc.Server) {
	grpcServer.RegisterService(serviceDesc, s)
}

func (s *Service) Health(_ context.Context, _ *HealthRequest) (*HealthResponse, error) {
	return &HealthResponse{
		Status: "ok",
		Time:   time.Now().UTC(),
	}, nil
}

func (s *Service) ListRuns(_ context.Context, req *ListRunsRequest) (*ListRunsResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	items, err := state.New(s.baseDir).ListRuns(limit)
	if err != nil {
		return nil, err
	}
	return &ListRunsResponse{
		Count: len(items),
		Items: items,
	}, nil
}

func Listen(addr, baseDir string) (*grpc.Server, net.Listener, error) {
	if addr == "" {
		return nil, nil, errors.New("grpc addr is required")
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	grpcServer := grpc.NewServer()
	New(baseDir).Register(grpcServer)
	return grpcServer, lis, nil
}

var serviceDesc = &grpc.ServiceDesc{
	ServiceName: "masterchef.v1.Control",
	HandlerType: (*ControlServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Health",
			Handler:    healthHandler,
		},
		{
			MethodName: "ListRuns",
			Handler:    listRunsHandler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "masterchef.v1",
}

func healthHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(HealthRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ControlServer).Health(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/masterchef.v1.Control/Health",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(ControlServer).Health(ctx, req.(*HealthRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func listRunsHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(ListRunsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ControlServer).ListRuns(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/masterchef.v1.Control/ListRuns",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(ControlServer).ListRuns(ctx, req.(*ListRunsRequest))
	}
	return interceptor(ctx, in, info, handler)
}
