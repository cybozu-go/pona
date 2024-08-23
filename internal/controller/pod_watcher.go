package controller

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"

	"github.com/cybozu-go/pona/internal/tunnel"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	EgressAnnotationPrefix = "egress.pona.cybozu.com/"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	EgressName      string
	EgressNamespace string

	linkMutex sync.Mutex

	podToPodIPs map[types.NamespacedName][]netip.Addr
	podIPToPod  map[netip.Addr]Set[types.NamespacedName]

	tun tunnel.Tunnel
}

type Set[T comparable] map[T]struct{}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.handlePodDeletion(ctx, req.NamespacedName); err != nil {
				logger.Error(err, "failed to remove tunnel")
				return ctrl.Result{}, fmt.Errorf("failed to remove tunnel: %w", err)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Pod: %w", err)
	}

	if !r.shouldHandle(pod) {
		return ctrl.Result{}, nil
	}

	// Pod is terminated or terminating
	if isTerminated(pod) || pod.DeletionTimestamp != nil {
		if err := r.handlePodDeletion(ctx, req.NamespacedName); err != nil {
			logger.Error(err, "failed to remove tunnel for terminated pod")
			return ctrl.Result{}, fmt.Errorf("failed to remove tunnel for terminated pod: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if err := r.handlePodRunning(ctx, pod); err != nil {
		logger.Error(err, "failed to setup tunnel")
		return ctrl.Result{}, fmt.Errorf("failed to setup tunnel: %w", err)
	}

	return ctrl.Result{}, nil
}

func isTerminated(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

func (r *PodReconciler) shouldHandle(pod *corev1.Pod) bool {
	if pod.Spec.HostNetwork {
		// Egress feature is not available for Pods running in the host network.
		return false
	}

	return r.hasEgressAnnotation(pod)
}

func (r *PodReconciler) handlePodRunning(ctx context.Context, pod *corev1.Pod) error {
	logger := log.FromContext(ctx)

	r.linkMutex.Lock()
	defer r.linkMutex.Unlock()

	podKey := types.NamespacedName{
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}

	existing := r.podToPodIPs[podKey]
	statusPodIPs := make([]netip.Addr, len(pod.Status.PodIPs))
	for i, v := range pod.Status.PodIPs {
		addr, err := netip.ParseAddr(v.IP)
		if err != nil {
			return err
		}
		statusPodIPs[i] = addr
	}

	for _, ip := range statusPodIPs {
		if slices.Contains(existing, ip) {
			continue
		}

		if err := r.tun.Add(ip); err != nil {
			if errors.Is(err, tunnel.ErrIPFamilyMismatch) {
				logger.Info("skipping unsupported pod IP", "pod", podKey, "ip", ip.String())
				continue
			}
			return err
		}
	}

	for _, eip := range existing {
		if slices.Contains(statusPodIPs, eip) {
			continue
		}

		if err := r.tun.Del(eip); err != nil {
			return err
		}
		logger.Info("tunnel has been deleted",
			"caller", "addPod",
			"pod", podKey,
			"ip", eip.String(),
		)
	}

	r.podToPodIPs[podKey] = statusPodIPs
	for _, ip := range statusPodIPs {
		keySet, ok := r.podIPToPod[ip]
		if !ok {
			r.podIPToPod[ip] = make(Set[types.NamespacedName], 0)
		}
		keySet[podKey] = struct{}{}
	}

	return nil
}

// TODO
func (r *PodReconciler) handlePodDeletion(ctx context.Context, namespacedName types.NamespacedName) error {
	logger := log.FromContext(ctx)

	r.linkMutex.Lock()
	defer r.linkMutex.Unlock()
	for _, ip := range r.podToPodIPs[namespacedName] {
		exists, err := r.existsOtherLiveTunnels(namespacedName, ip)
		if err != nil {
			return err
		}

		if !exists {
			if err := r.tun.Del(ip); err != nil {
				return err
			}

			logger.Info("tunnel has been deleted",
				"caller", "addPod",
				"pod", namespacedName,
				"ip", ip.String(),
			)
		}

		if keySet, ok := r.podIPToPod[ip]; ok {
			delete(keySet, namespacedName)
			if len(keySet) == 0 {
				delete(r.podIPToPod, ip)
			}
		}
	}

	delete(r.podToPodIPs, namespacedName)

	return nil
}

func (r *PodReconciler) existsOtherLiveTunnels(namespacedName types.NamespacedName, ip netip.Addr) (bool, error) {
	if keySet, ok := r.podIPToPod[ip]; ok {
		if _, ok := keySet[namespacedName]; ok {
			return len(keySet) > 1, nil
		}
		return false, fmt.Errorf("keySet in the podIPToPod doesn't contain my key. key: %s ip: %s", namespacedName, ip)
	}

	return false, fmt.Errorf("podIPToPod doesn't contain my IP. key: %s ip: %s", namespacedName, ip)
}

func (r *PodReconciler) hasEgressAnnotation(pod *corev1.Pod) bool {
	for k, name := range pod.Annotations {
		if !strings.HasPrefix(k, EgressAnnotationPrefix) {
			continue
		}

		if k[len(EgressAnnotationPrefix):] != r.EgressNamespace {
			continue
		}

		// shortcut for the most typical case
		if name == r.EgressName {
			return true
		}

		for _, n := range strings.Split(name, ",") {
			if n == r.EgressNamespace {
				return true
			}
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
