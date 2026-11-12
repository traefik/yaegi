package grpcplug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"example.com/complex/grpc/eventpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// The service below is what protoc-gen-go-grpc generates for
// "service Echo { rpc Ping(Event) returns (Event); }".

type echoServer interface {
	Ping(context.Context, *eventpb.Event) (*eventpb.Event, error)
}

func pingHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(eventpb.Event)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(echoServer).Ping(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/complex.v1.Echo/Ping"}
	h := func(ctx context.Context, req any) (any, error) {
		return srv.(echoServer).Ping(ctx, req.(*eventpb.Event))
	}
	return interceptor(ctx, in, info, h)
}

var echoServiceDesc = grpc.ServiceDesc{
	ServiceName: "complex.v1.Echo",
	HandlerType: (*echoServer)(nil),
	Methods:     []grpc.MethodDesc{{MethodName: "Ping", Handler: pingHandler}},
	Metadata:    "complex/v1/event.proto",
}

type echo struct{ calls atomic.Int64 }

func (e *echo) Ping(ctx context.Context, in *eventpb.Event) (*eventpb.Event, error) {
	e.calls.Add(1)
	md, _ := metadata.FromIncomingContext(ctx)
	if strings.Contains(in.GetId(), "/admin") {
		return nil, status.Errorf(codes.PermissionDenied, "admin path %q", in.GetId())
	}
	out := &eventpb.Event{
		Id:     "pong:" + in.GetId(),
		Ts:     in.GetTs() + 1,
		Labels: map[string]string{"client": strings.Join(md.Get("x-client"), ",")},
		Kind:   in.GetKind(),
	}
	return out, nil
}

type state struct {
	conn   *grpc.ClientConn
	srv    *grpc.Server
	health healthpb.HealthClient
	echo   *echo
	seen   atomic.Int64 // calls seen by the client interceptor
}

func setup(ctx context.Context) (*state, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &state{echo: &echo{}}
	s.srv = grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		resp, err := h(ctx, req)
		grpc.SetHeader(ctx, metadata.Pairs("x-method", info.FullMethod))
		return resp, err
	}))
	s.srv.RegisterService(&echoServiceDesc, s.echo)
	hs := health.NewServer()
	hs.SetServingStatus("complex.v1.Echo", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(s.srv, hs)
	go s.srv.Serve(lis)

	s.conn, err = grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			s.seen.Add(1)
			return invoker(metadata.AppendToOutgoingContext(ctx, "x-client", "traefik"), method, req, reply, cc, opts...)
		}))
	if err != nil {
		return nil, err
	}
	s.health = healthpb.NewHealthClient(s.conn)
	go func() {
		<-ctx.Done()
		s.conn.Close()
		s.srv.Stop()
	}()
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()
	hr, err := s.health.Check(ctx, &healthpb.HealthCheckRequest{Service: "complex.v1.Echo"})
	if err != nil {
		return "", err
	}
	in := &eventpb.Event{Id: req.Method + " " + req.URL.Path, Ts: 41, Kind: eventpb.Kind_KIND_READ}
	out := new(eventpb.Event)
	var hdr metadata.MD
	err = s.conn.Invoke(ctx, "/complex.v1.Echo/Ping", in, out, grpc.Header(&hdr))
	if err != nil {
		st, _ := status.FromError(err)
		return fmt.Sprintf("health=%s code=%s msg=%s", hr.GetStatus(), st.Code(), st.Message()), nil
	}
	return fmt.Sprintf("health=%s id=%s ts=%d client=%s method=%s calls=%d/%d",
		hr.GetStatus(), out.GetId(), out.GetTs(), out.GetLabels()["client"], hdr.Get("x-method"),
		s.echo.calls.Load(), s.seen.Load()), nil
}
