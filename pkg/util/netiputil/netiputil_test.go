package netiputil

import (
	"net"
	"net/netip"
	"reflect"
	"testing"
)

func TestToIPNet(t *testing.T) {
	type args struct {
		prefix netip.Prefix
	}
	tests := []struct {
		name string
		args args
		want net.IPNet
	}{
		{
			name: "192.168.0.0/16",
			args: args{
				prefix: netip.MustParsePrefix("192.168.0.0/16"),
			},
			want: net.IPNet{
				IP: net.ParseIP("192.168.0.0").To4(), Mask: net.CIDRMask(16, 32),
			},
		},
		{
			name: "fc00::/7",
			args: args{
				prefix: netip.MustParsePrefix("fc00::/7"),
			},
			want: net.IPNet{
				IP:   net.ParseIP("fc00::"),
				Mask: net.CIDRMask(7, 128),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToIPNet(tt.args.prefix); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToIPNet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromIPNet(t *testing.T) {
	type args struct {
		from net.IPNet
	}
	tests := []struct {
		name   string
		args   args
		wantTo netip.Prefix
		wantOk bool
	}{
		{
			name: "192.168.0.0/16",
			args: args{
				from: net.IPNet{
					IP: net.ParseIP("192.168.0.0").To4(), Mask: net.CIDRMask(16, 32),
				},
			},
			wantTo: netip.MustParsePrefix("192.168.0.0/16"),
			wantOk: true,
		},
		{
			name: "fc00::/7",
			args: args{
				from: net.IPNet{
					IP:   net.ParseIP("fc00::"),
					Mask: net.CIDRMask(7, 128),
				},
			},
			wantTo: netip.MustParsePrefix("fc00::/7"),
			wantOk: true,
		},
		{
			name: "invalid",
			args: args{
				from: net.IPNet{},
			},
			wantTo: netip.Prefix{},
			wantOk: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTo, gotOk := FromIPNet(tt.args.from)
			if !reflect.DeepEqual(gotTo, tt.wantTo) {
				t.Errorf("FromIPNet() gotTo = %v, want %v", gotTo, tt.wantTo)
			}
			if gotOk != tt.wantOk {
				t.Errorf("FromIPNet() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func TestFromAddr(t *testing.T) {
	type args struct {
		addr netip.Addr
	}
	tests := []struct {
		name string
		args args
		want net.IP
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromAddr(tt.args.addr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromAddr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToAddr(t *testing.T) {
	type args struct {
		ip net.IP
	}
	tests := []struct {
		name string
		args args
		want netip.Addr
	}{
		{
			name: "1",
			args: args{
				ip: net.ParseIP("10.244.0.1"),
			},
			want: netip.MustParseAddr("10.244.0.1"),
		},
		{
			name: "2",
			args: args{
				ip: net.ParseIP("ffcc::1"),
			},
			want: netip.MustParseAddr("ffcc::1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := ToAddr(tt.args.ip)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToAddr() got = %v, want %v", got, tt.want)
			}
			if !got1 {
				t.Errorf("ToAddr() got1 = %v, want true", got1)
			}
		})
	}
}
