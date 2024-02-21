package operator

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/b3scale/b3scale-operator/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
)

type OperatorKubernetesClient struct {
	clientset *kubernetes.Clientset
}

func NewOperatorKubernetesClient(cs *kubernetes.Clientset) *OperatorKubernetesClient {
	okc := OperatorKubernetesClient{clientset: cs}
	return &okc
}

func (o *OperatorKubernetesClient) UpdateBBBFrontendStatus(ctx context.Context, bbb *v1.BBBFrontend) error {
	jsonBody, err := json.Marshal(bbb)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("/apis/b3scale.infra.run/v1/namespaces/%v/bbbfrontends/%v/status", bbb.Namespace, bbb.Name)
	d := o.clientset.RESTClient().Put().AbsPath(url).Body(jsonBody).Do(ctx)

	err = d.Error()
	if err != nil {
		return err
	}

	return nil
}

func (o *OperatorKubernetesClient) UpdateBBBFrontend(ctx context.Context, bbb *v1.BBBFrontend) error {
	jsonBody, err := json.Marshal(bbb)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("/apis/b3scale.infra.run/v1/namespaces/%v/bbbfrontends/%v", bbb.Namespace, bbb.Name)
	d := o.clientset.RESTClient().Put().AbsPath(url).Body(jsonBody).Do(ctx)
	if d.Error() != nil {
		return d.Error()
	}

	return nil
}

func (o *OperatorKubernetesClient) AddFinalizerToBBBFrontend(ctx context.Context, bbb *v1.BBBFrontend, finalizer string) error {
	var finalizers []string
	copy(finalizers, bbb.Finalizers)
	finalizers = append(finalizers, finalizer)
	bbb.SetFinalizers(finalizers)
	return o.UpdateBBBFrontend(ctx, bbb)
}

func (o *OperatorKubernetesClient) RemoveFinalizerFromBBBFrontend(ctx context.Context, bbb *v1.BBBFrontend, finalizer string) error {
	// We want to remove our finalizer, but keep the other finalizers.
	finalizers := []string{}
	for _, f := range bbb.Finalizers {
		if f != finalizer {
			finalizers = append(finalizers, f)
		}
	}

	bbb.SetFinalizers(finalizers)
	return o.UpdateBBBFrontend(ctx, bbb)
}

func (o *OperatorKubernetesClient) RemoveFinalizerFromConfigMap(ctx context.Context, configMap *corev1.ConfigMap, finalizer string) error {
	// We want to remove our finalizer, but keep the other finalizers.
	finalizers := []string{}
	for _, f := range configMap.Finalizers {
		if f != finalizer {
			finalizers = append(finalizers, f)
		}
	}

	configMap.SetFinalizers(finalizers)

	_, err := o.clientset.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	return err
}

func (o *OperatorKubernetesClient) SetReadyStatusCondition(ctx context.Context, bbb *v1.BBBFrontend, isReady bool, err error) error {

	var status metav1.ConditionStatus
	var message string
	var reason string
	if isReady {
		status = metav1.ConditionTrue
		reason = "SuccessReconcile"
		message = "Resource was successfully reconciled"

	} else {
		status = metav1.ConditionFalse
		reason = "ErrReconcile"
		message = err.Error()
	}

	metaNow := metav1.NewTime(time.Now())

	var existingReadyCondition *metav1.Condition
	for _, c := range bbb.Status.Conditions {
		con := c
		if c.Type == "Ready" {
			existingReadyCondition = &con
		}
	}

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: bbb.GetGeneration(),
		LastTransitionTime: metaNow,
		Reason:             reason,
		Message:            message,
	}

	if existingReadyCondition != nil &&
		existingReadyCondition.Status == condition.Status &&
		existingReadyCondition.ObservedGeneration == condition.ObservedGeneration &&
		existingReadyCondition.Reason == condition.Reason &&
		existingReadyCondition.Message == condition.Message {

		// Skip the update, we do not need it.
		return nil
	}

	var conditions []metav1.Condition

	for _, f := range bbb.Status.Conditions {
		if f.Type != condition.Type {
			conditions = append(conditions, f)
		}
	}

	conditions = append(conditions, condition)
	bbb.Status.Conditions = conditions

	return o.UpdateBBBFrontendStatus(ctx, bbb)
}
