// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.27.3
// source: pkg/cnirpc/cni.proto

package cnirpc

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
	CNI_Add_FullMethodName   = "/pkg.cnirpc.CNI/Add"
	CNI_Del_FullMethodName   = "/pkg.cnirpc.CNI/Del"
	CNI_Check_FullMethodName = "/pkg.cnirpc.CNI/Check"
)

// CNIClient is the client API for CNI service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// CNI implements CNI commands over gRPC.
type CNIClient interface {
	Add(ctx context.Context, in *CNIArgs, opts ...grpc.CallOption) (*AddResponse, error)
	Del(ctx context.Context, in *CNIArgs, opts ...grpc.CallOption) (*emptypb.Empty, error)
	Check(ctx context.Context, in *CNIArgs, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type cNIClient struct {
	cc grpc.ClientConnInterface
}

func NewCNIClient(cc grpc.ClientConnInterface) CNIClient {
	return &cNIClient{cc}
}

func (c *cNIClient) Add(ctx context.Context, in *CNIArgs, opts ...grpc.CallOption) (*AddResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(AddResponse)
	err := c.cc.Invoke(ctx, CNI_Add_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *cNIClient) Del(ctx context.Context, in *CNIArgs, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, CNI_Del_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *cNIClient) Check(ctx context.Context, in *CNIArgs, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, CNI_Check_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CNIServer is the server API for CNI service.
// All implementations must embed UnimplementedCNIServer
// for forward compatibility.
//
// CNI implements CNI commands over gRPC.
type CNIServer interface {
	Add(context.Context, *CNIArgs) (*AddResponse, error)
	Del(context.Context, *CNIArgs) (*emptypb.Empty, error)
	Check(context.Context, *CNIArgs) (*emptypb.Empty, error)
	mustEmbedUnimplementedCNIServer()
}

// UnimplementedCNIServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedCNIServer struct{}

func (UnimplementedCNIServer) Add(context.Context, *CNIArgs) (*AddResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Add not implemented")
}
func (UnimplementedCNIServer) Del(context.Context, *CNIArgs) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Del not implemented")
}
func (UnimplementedCNIServer) Check(context.Context, *CNIArgs) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Check not implemented")
}
func (UnimplementedCNIServer) mustEmbedUnimplementedCNIServer() {}
func (UnimplementedCNIServer) testEmbeddedByValue()             {}

// UnsafeCNIServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to CNIServer will
// result in compilation errors.
type UnsafeCNIServer interface {
	mustEmbedUnimplementedCNIServer()
}

func RegisterCNIServer(s grpc.ServiceRegistrar, srv CNIServer) {
	// If the following call pancis, it indicates UnimplementedCNIServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&CNI_ServiceDesc, srv)
}

func _CNI_Add_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CNIArgs)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CNIServer).Add(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: CNI_Add_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CNIServer).Add(ctx, req.(*CNIArgs))
	}
	return interceptor(ctx, in, info, handler)
}

func _CNI_Del_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CNIArgs)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CNIServer).Del(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: CNI_Del_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CNIServer).Del(ctx, req.(*CNIArgs))
	}
	return interceptor(ctx, in, info, handler)
}

func _CNI_Check_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CNIArgs)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CNIServer).Check(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: CNI_Check_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CNIServer).Check(ctx, req.(*CNIArgs))
	}
	return interceptor(ctx, in, info, handler)
}

// CNI_ServiceDesc is the grpc.ServiceDesc for CNI service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var CNI_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "pkg.cnirpc.CNI",
	HandlerType: (*CNIServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Add",
			Handler:    _CNI_Add_Handler,
		},
		{
			MethodName: "Del",
			Handler:    _CNI_Del_Handler,
		},
		{
			MethodName: "Check",
			Handler:    _CNI_Check_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "pkg/cnirpc/cni.proto",
}
