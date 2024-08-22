package controller

import (
	"context"
	"fmt"
	"time"

	ponav1beta1 "github.com/cybozu-go/pona/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Egress Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"
		const namespace = "default"

		ctx := context.Background()

		namespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}
		egress := &ponav1beta1.Egress{}

		desiredEgress := &ponav1beta1.Egress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: ponav1beta1.EgressSpec{
				Destinations: []string{
					"10.0.0.0/8",
				},
				Replicas: 3,
				Strategy: &appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxUnavailable: ptr.To(intstr.FromInt(2)),
						MaxSurge:       ptr.To(intstr.FromInt(0)),
					},
				},
				Template: &ponav1beta1.EgressPodTemplate{
					Metadata: ponav1beta1.Metadata{
						Annotations: map[string]string{
							"ann1": "foo",
						},
						Labels: map[string]string{
							"label1": "bar",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "egress",
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										"memory": resource.MustParse("400Mi"),
									},
								},
							},
						},
					},
				},
				SessionAffinity: corev1.ServiceAffinityClientIP,
				SessionAffinityConfig: &corev1.SessionAffinityConfig{
					ClientIP: &corev1.ClientIPConfig{
						TimeoutSeconds: ptr.To(int32(43200)),
					},
				},
				PodDisruptionBudget: &ponav1beta1.EgressPDBSpec{
					MaxUnavailable: ptr.To(intstr.FromInt(1)),
				},
			},
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Egress")
			err := k8sClient.Get(ctx, namespacedName, egress)
			if apierrors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, desiredEgress)).To(Succeed())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			resource := &ponav1beta1.Egress{}
			err := k8sClient.Get(ctx, namespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Egress")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

		})
		It("should successfully reconcile the resource", func() {
			const port = 5555

			By("Reconciling the created resource")
			controllerReconciler := &EgressReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),

				Port: port,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Check if ServiceAccount is created")
			var sa *corev1.ServiceAccount
			Eventually(func() error {
				sa = &corev1.ServiceAccount{}
				return k8sClient.Get(ctx, client.ObjectKey{Name: egressServiceAccountName, Namespace: namespace}, sa)
			}).Should(Succeed())

			By("Check if ClusterRole is created")
			var cr *rbacv1.ClusterRole
			Eventually(func() error {
				cr = &rbacv1.ClusterRole{}
				return k8sClient.Get(ctx, client.ObjectKey{Name: egressCRName, Namespace: namespace}, cr)
			}).Should(Succeed())
			Expect(cr.Rules).To(Equal([]rbacv1.PolicyRule{{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			}}))

			By("Check if ClusterRoleBinding is created")
			var crb *rbacv1.ClusterRoleBinding
			Eventually(func() error {
				crb = &rbacv1.ClusterRoleBinding{}
				return k8sClient.Get(ctx, client.ObjectKey{Name: egressCRBName, Namespace: namespace}, crb)
			}).Should(Succeed())
			Expect(crb.Subjects).To(Equal([]rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      egressServiceAccountName,
				Namespace: namespace,
			}}))

			By("Check if Deployment is created")
			var dep *appsv1.Deployment
			Eventually(func() error {
				dep = &appsv1.Deployment{}
				return k8sClient.Get(ctx, client.ObjectKey{Name: desiredEgress.Name, Namespace: namespace}, dep)
			}).Should(Succeed())
			Expect(dep.OwnerReferences).To(HaveLen(1))
			Expect(dep.Spec.Replicas).NotTo(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(desiredEgress.Spec.Replicas))

			Expect(dep.Spec.Template.Labels).To(HaveKeyWithValue(labelAppName, "pona"))
			Expect(dep.Spec.Template.Labels).To(HaveKeyWithValue(labelAppComponent, "egress"))
			Expect(dep.Spec.Template.Labels).To(HaveKeyWithValue(labelAppInstance, desiredEgress.Name))
			Expect(dep.Spec.Template.Spec.ServiceAccountName).To(Equal(egressServiceAccountName))
			Expect(dep.Spec.Template.Spec.Volumes).To(HaveLen(2))

			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			egressContainer := dep.Spec.Template.Spec.Containers[0]

			Expect(egressContainer).NotTo(BeNil())
			Expect(egressContainer.Image).To(Equal(egressImage))
			Expect(egressContainer.Command).To(Equal([]string{"coil-egress"})) //TODO: Change this when use another container image
			Expect(egressContainer.Env).To(HaveLen(3))
			Expect(egressContainer.VolumeMounts).To(HaveLen(2))
			Expect(egressContainer.SecurityContext).NotTo(BeNil())
			Expect(egressContainer.SecurityContext.ReadOnlyRootFilesystem).NotTo(BeNil())
			Expect(*egressContainer.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
			Expect(egressContainer.SecurityContext.Privileged).NotTo(BeNil())
			Expect(*egressContainer.SecurityContext.Privileged).To(BeTrue())
			Expect(egressContainer.SecurityContext.Capabilities).NotTo(BeNil())
			Expect(*egressContainer.SecurityContext.Capabilities).To(Equal(corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN"}}))
			Expect(egressContainer.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceCPU, resource.MustParse(egressDefaultCpuRequest)))
			Expect(egressContainer.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceMemory, resource.MustParse(egressDefaultMemRequest)))
			Expect(egressContainer.Ports).To(Equal(
				[]corev1.ContainerPort{
					{Name: "metrics", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
					{Name: "health", ContainerPort: 8081, Protocol: corev1.ProtocolTCP},
				},
			))
			Expect(egressContainer.LivenessProbe).NotTo(BeNil())
			Expect(egressContainer.LivenessProbe.ProbeHandler).To(Equal(
				corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
					Path:   "/livez",
					Port:   intstr.FromString("livez"),
					Scheme: corev1.URISchemeHTTP,
				}},
			))
			Expect(egressContainer.ReadinessProbe).NotTo(BeNil())
			Expect(egressContainer.ReadinessProbe.ProbeHandler).To(Equal(
				corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
					Path:   "/readyz",
					Port:   intstr.FromString("health"),
					Scheme: corev1.URISchemeHTTP,
				}},
			))

			By("Check if Service is created")
			var svc *corev1.Service
			Eventually(func() error {
				svc = &corev1.Service{}
				return k8sClient.Get(ctx, client.ObjectKey(namespacedName), svc)
			}).Should(Succeed())
			Expect(svc.OwnerReferences).To(HaveLen(1))

			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue(labelAppName, "pona"))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue(labelAppComponent, "egress"))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue(labelAppInstance, desiredEgress.Name))

			Expect(svc.Spec.Ports).To(Equal(
				[]corev1.ServicePort{{
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   corev1.ProtocolUDP,
				}},
			))
			Expect(svc.Spec.SessionAffinity).To(Equal(desiredEgress.Spec.SessionAffinity))
			Expect(svc.Spec.SessionAffinityConfig).To(Equal(desiredEgress.Spec.SessionAffinityConfig))

			By("Check if PodDisruptionBudget is created")
			var pdb *policyv1.PodDisruptionBudget
			Eventually(func() error {
				pdb = &policyv1.PodDisruptionBudget{}
				return k8sClient.Get(ctx, client.ObjectKey(namespacedName), pdb)
			}).Should(Succeed())
			Expect(pdb.OwnerReferences).To(HaveLen(1))

			Expect(pdb.Labels).To(HaveKeyWithValue(labelAppName, "pona"))
			Expect(pdb.Labels).To(HaveKeyWithValue(labelAppComponent, "egress"))
			Expect(pdb.Labels).To(HaveKeyWithValue(labelAppInstance, desiredEgress.Name))

			Expect(pdb.Spec.MaxUnavailable).To(Equal(desiredEgress.Spec.PodDisruptionBudget.MaxUnavailable))

			Expect(pdb.Spec.Selector).To(HaveKeyWithValue(labelAppName, "pona"))
			Expect(pdb.Spec.Selector).To(HaveKeyWithValue(labelAppComponent, "egress"))
			Expect(pdb.Spec.Selector).To(HaveKeyWithValue(labelAppInstance, desiredEgress.Name))

			By("Delete egress")
			Expect(k8sClient.Delete(ctx, desiredEgress)).NotTo(HaveOccurred())
			Expect(func() error {
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: namespacedName,
				})
				return err
			}).NotTo(HaveOccurred())

			const timeout = 5 * time.Second
			By("Check if ServiceAccount is not deleted")
			Consistently(func() error {
				sa := &corev1.ServiceAccount{}
				return k8sClient.Get(ctx, client.ObjectKey{Name: egressServiceAccountName, Namespace: namespace}, sa)
			}).WithTimeout(timeout).Should(Succeed())

			By("Check if ClusterRole is not deleted")
			Consistently(func() error {
				cr := &rbacv1.ClusterRole{}
				return k8sClient.Get(ctx, client.ObjectKey{Name: egressCRName, Namespace: namespace}, cr)
			}).WithTimeout(timeout).Should(Succeed())

			By("Check if ClusterRoleBinding is not deleted")
			Consistently(func() error {
				crb := &rbacv1.ClusterRoleBinding{}
				return k8sClient.Get(ctx, client.ObjectKey{Name: egressCRBName, Namespace: namespace}, crb)
			}).WithTimeout(timeout).Should(Succeed())

			By("Check if Deployment is deleted")
			Eventually(func() error {
				dep := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, client.ObjectKey(namespacedName), dep)
				return isNotfound(err, dep)
			}).Should(Succeed())

			By("Check if Service is deleted")
			Eventually(func() error {
				svc := &corev1.Service{}
				err := k8sClient.Get(ctx, client.ObjectKey(namespacedName), svc)
				return isNotfound(err, svc)
			}).Should(Succeed())

			By("Check if PodDisruptionBudget is deleted")
			Eventually(func() error {
				pdb := &policyv1.PodDisruptionBudget{}
				err := k8sClient.Get(ctx, client.ObjectKey(namespacedName), pdb)
				return isNotfound(err, pdb)
			}).Should(Succeed())
		})
	})
})

func isNotfound(err error, resource client.Object) error {
	if apierrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	return fmt.Errorf("%s still exists", resource.GetName())
}
