package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ponav1beta1 "github.com/cybozu-go/pona/api/v1beta1"
)

var _ = Describe("Egress Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"
		const namespace = "default"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}
		egress := &ponav1beta1.Egress{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Egress")
			maxUnavailable := intstr.FromInt(1)
			const timeoutSeconds = int32(43200)

			err := k8sClient.Get(ctx, typeNamespacedName, egress)
			if err != nil && errors.IsNotFound(err) {
				resource := &ponav1beta1.Egress{
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
								MaxUnavailable: 2,
								MaxSurge:       0,
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
								TimeoutSeconds: ptr.To(timeoutSeconds),
							},
						},
						PodDisruptionBudget: &ponav1beta1.EgressPDBSpec{
							MaxUnavailable: &maxUnavailable,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &ponav1beta1.Egress{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Egress")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Check if ServiceAccount is deleted")
			By("Check if ClusterRole is not deleted")
			By("Check if ClusterRoleBinding is not deleted")
			By("Check if Deployment is deleted")
			By("Check if PodDisruptionBudget is deleted")
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &EgressReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Check if ServiceAccount is created")
			var sa *corev1.ServiceAccount
			Eventually(func() error {
				sa = &corev1.ServiceAccount{}
				return k8sClient.Get(ctx, client.ObjectKey{})
			})
			By("Check if ClusterRole is created")
			By("Check if ClusterRoleBinding is created")
			By("Check if Deployment is created")
			By("Check if PodDisruptionBudget is created")
		})
	})
})
