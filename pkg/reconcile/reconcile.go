package reconcile

import (
	"context"
	"github.com/thcyron/skop/v2/reconcile"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

/**
This implements missing reconcile functions in skop.
*/

func Secret(ctx context.Context, cs *kubernetes.Clientset, secret *corev1.Secret) error {
	client := cs.CoreV1().Secrets(secret.Namespace)
	existing, err := client.Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = client.Create(ctx, secret, metav1.CreateOptions{})
			return err
		}
		return err
	}
	existing.Labels = secret.Labels
	existing.Annotations = secret.Annotations
	existing.StringData = secret.StringData
	existing.Data = secret.Data
	existing.Immutable = secret.Immutable
	_, err = client.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func SecretAbsence(ctx context.Context, cs *kubernetes.Clientset, configMap *corev1.ConfigMap) error {
	return reconcile.Absence(func() error {
		return cs.CoreV1().ConfigMaps(configMap.Namespace).Delete(ctx, configMap.Name, metav1.DeleteOptions{})
	})
}
