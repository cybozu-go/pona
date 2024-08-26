package controller

import (
	"context"
	"net/netip"
	"path/filepath"

	"github.com/cybozu-go/pona/internal/tunnel"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockTunnel struct {
	tunnels map[netip.Addr]struct{}
}

var _ tunnel.Controller = &mockTunnel{}

func NewMockTunnel() mockTunnel {
	return mockTunnel{
		tunnels: make(map[netip.Addr]struct{}),
	}
}

func (m mockTunnel) AddPeer(addr netip.Addr) (netlink.Link, error) {
	m.tunnels[addr] = struct{}{}
	return nil, nil
}

func (m mockTunnel) DelPeer(addr netip.Addr) error {
	delete(m.tunnels, addr)
	return nil
}

func (m mockTunnel) Init() error {
	return nil
}

func (m mockTunnel) IsInitialized() bool {
	return true
}

type pod struct {
	NamespacedName types.NamespacedName
	PodIPs         []netip.Addr
}

var _ = Describe("Pod Watcher", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		const (
			egressName      = "egress"
			egressNamespace = "default"
		)

		podInfo := pod{
			NamespacedName: types.NamespacedName{
				Name:      "pod",
				Namespace: "default",
			},
			PodIPs: []netip.Addr{netip.MustParseAddr("192.168.0.1")},
		}
		pod := &corev1.Pod{}

		BeforeEach(func() {
			pod.SetName(podInfo.NamespacedName.Name)
			pod.SetNamespace(podInfo.NamespacedName.Namespace)
			pod.Spec.Containers = []corev1.Container{
				{
					Name:  "container",
					Image: "image",
				},
			}

			pod.Annotations = map[string]string{
				filepath.Join(EgressAnnotationPrefix, egressNamespace): egressName,
			}

			By("create pod")
			err := k8sClient.Create(ctx, pod)
			Expect(err).NotTo(HaveOccurred())

			By("set pod status")
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey(podInfo.NamespacedName), pod)
			}).Should(Succeed())

			pod.Status.PodIPs = make([]corev1.PodIP, 0, len(podInfo.PodIPs))
			for _, ip := range podInfo.PodIPs {
				pod.Status.PodIPs = append(pod.Status.PodIPs, corev1.PodIP{IP: ip.String()})
			}

			err = k8sClient.Status().Update(ctx, pod)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
		})

		It("should successfully reconcile the resource", func() {
			By("Reconcile the created resource")
			t := NewMockTunnel()
			w := &PodWatcher{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				tun:    t,

				EgressName:      egressName,
				EgressNamespace: egressNamespace,

				podToPodIPs: make(map[types.NamespacedName][]netip.Addr),
				podIPToPod:  make(map[netip.Addr]Set[types.NamespacedName]),
			}

			_, err := w.Reconcile(ctx, reconcile.Request{
				NamespacedName: podInfo.NamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Check if Add() is called")
			for _, ip := range podInfo.PodIPs {
				_, ok := t.tunnels[ip]
				Expect(ok).To(BeTrue())
			}

			By("Check podToPodIPs, podIPsToPod")
			Expect(w.podToPodIPs).To(Equal(map[types.NamespacedName][]netip.Addr{
				podInfo.NamespacedName: podInfo.PodIPs,
			}))
			Expect(w.podIPToPod).To(Equal(podIPToPod(podInfo)))

			By("Delete Pod")

			err = k8sClient.Delete(ctx, pod)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the created resource")
			_, err = w.Reconcile(ctx, reconcile.Request{
				NamespacedName: podInfo.NamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Check if Delete() is called")
			for _, ip := range podInfo.PodIPs {
				_, ok := t.tunnels[ip]
				Expect(ok).To(BeFalse())
			}

			By("Check podToPodIPs, podIPsToPod")
			Expect(w.podToPodIPs).To(Equal(map[types.NamespacedName][]netip.Addr{}))
			Expect(w.podIPToPod).To(Equal(map[netip.Addr]Set[types.NamespacedName]{}))

		})
	})
})

func podIPToPod(pod pod) map[netip.Addr]Set[types.NamespacedName] {
	m := make(map[netip.Addr]Set[types.NamespacedName])
	for _, v := range pod.PodIPs {
		nns, ok := m[v]
		if !ok {
			m[v] = Set[types.NamespacedName]{
				pod.NamespacedName: struct{}{},
			}
		} else {
			nns[pod.NamespacedName] = struct{}{}
		}
	}
	return m
}
