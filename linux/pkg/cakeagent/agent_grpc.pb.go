// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.29.3
// source: agent.proto

package cakeagent

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Agent_Info_FullMethodName    = "/cakeagent.Agent/Info"
	Agent_Execute_FullMethodName = "/cakeagent.Agent/Execute"
	Agent_Shell_FullMethodName   = "/cakeagent.Agent/Shell"
	Agent_Mount_FullMethodName   = "/cakeagent.Agent/Mount"
	Agent_Umount_FullMethodName  = "/cakeagent.Agent/Umount"
)

// AgentClient is the client API for Agent service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type AgentClient interface {
	Info(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*InfoReply, error)
	Execute(ctx context.Context, in *ExecuteRequest, opts ...grpc.CallOption) (*ExecuteReply, error)
	Shell(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[ShellMessage, ShellResponse], error)
	Mount(ctx context.Context, in *MountRequest, opts ...grpc.CallOption) (*MountReply, error)
	Umount(ctx context.Context, in *MountRequest, opts ...grpc.CallOption) (*MountReply, error)
}

type agentClient struct {
	cc grpc.ClientConnInterface
}

func NewAgentClient(cc grpc.ClientConnInterface) AgentClient {
	return &agentClient{cc}
}

func (c *agentClient) Info(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*InfoReply, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InfoReply)
	err := c.cc.Invoke(ctx, Agent_Info_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *agentClient) Execute(ctx context.Context, in *ExecuteRequest, opts ...grpc.CallOption) (*ExecuteReply, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ExecuteReply)
	err := c.cc.Invoke(ctx, Agent_Execute_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *agentClient) Shell(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[ShellMessage, ShellResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Agent_ServiceDesc.Streams[0], Agent_Shell_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[ShellMessage, ShellResponse]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Agent_ShellClient = grpc.BidiStreamingClient[ShellMessage, ShellResponse]

func (c *agentClient) Mount(ctx context.Context, in *MountRequest, opts ...grpc.CallOption) (*MountReply, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(MountReply)
	err := c.cc.Invoke(ctx, Agent_Mount_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *agentClient) Umount(ctx context.Context, in *MountRequest, opts ...grpc.CallOption) (*MountReply, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(MountReply)
	err := c.cc.Invoke(ctx, Agent_Umount_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AgentServer is the server API for Agent service.
// All implementations must embed UnimplementedAgentServer
// for forward compatibility.
type AgentServer interface {
	Info(context.Context, *emptypb.Empty) (*InfoReply, error)
	Execute(context.Context, *ExecuteRequest) (*ExecuteReply, error)
	Shell(grpc.BidiStreamingServer[ShellMessage, ShellResponse]) error
	Mount(context.Context, *MountRequest) (*MountReply, error)
	Umount(context.Context, *MountRequest) (*MountReply, error)
	mustEmbedUnimplementedAgentServer()
}

// UnimplementedAgentServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedAgentServer struct{}

func (UnimplementedAgentServer) Info(context.Context, *emptypb.Empty) (*InfoReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Info not implemented")
}
func (UnimplementedAgentServer) Execute(context.Context, *ExecuteRequest) (*ExecuteReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Execute not implemented")
}
func (UnimplementedAgentServer) Shell(grpc.BidiStreamingServer[ShellMessage, ShellResponse]) error {
	return status.Errorf(codes.Unimplemented, "method Shell not implemented")
}
func (UnimplementedAgentServer) Mount(context.Context, *MountRequest) (*MountReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Mount not implemented")
}
func (UnimplementedAgentServer) Umount(context.Context, *MountRequest) (*MountReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Umount not implemented")
}
func (UnimplementedAgentServer) mustEmbedUnimplementedAgentServer() {}
func (UnimplementedAgentServer) testEmbeddedByValue()               {}

// UnsafeAgentServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AgentServer will
// result in compilation errors.
type UnsafeAgentServer interface {
	mustEmbedUnimplementedAgentServer()
}

func RegisterAgentServer(s grpc.ServiceRegistrar, srv AgentServer) {
	// If the following call pancis, it indicates UnimplementedAgentServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Agent_ServiceDesc, srv)
}

func _Agent_Info_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServer).Info(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Agent_Info_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServer).Info(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _Agent_Execute_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ExecuteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServer).Execute(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Agent_Execute_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServer).Execute(ctx, req.(*ExecuteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Agent_Shell_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(AgentServer).Shell(&grpc.GenericServerStream[ShellMessage, ShellResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Agent_ShellServer = grpc.BidiStreamingServer[ShellMessage, ShellResponse]

func _Agent_Mount_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MountRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServer).Mount(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Agent_Mount_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServer).Mount(ctx, req.(*MountRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Agent_Umount_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MountRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServer).Umount(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Agent_Umount_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServer).Umount(ctx, req.(*MountRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Agent_ServiceDesc is the grpc.ServiceDesc for Agent service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Agent_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "cakeagent.Agent",
	HandlerType: (*AgentServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Info",
			Handler:    _Agent_Info_Handler,
		},
		{
			MethodName: "Execute",
			Handler:    _Agent_Execute_Handler,
		},
		{
			MethodName: "Mount",
			Handler:    _Agent_Mount_Handler,
		},
		{
			MethodName: "Umount",
			Handler:    _Agent_Umount_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Shell",
			Handler:       _Agent_Shell_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "agent.proto",
}
