package operator

import (
	"context"
	"encoding/json"
	"errors"

	"fmt"
	"os"
	"os/signal"
	"syscall"

	v1 "github.com/b3scale/b3scale-operator/pkg/apis/v1"
	config2 "github.com/b3scale/b3scale-operator/pkg/config"
	reconcile2 "github.com/b3scale/b3scale-operator/pkg/reconcile"
	"github.com/b3scale/b3scale-operator/pkg/util"
	"github.com/b3scale/b3scale/pkg/bbb"
	b3scaleclient "github.com/b3scale/b3scale/pkg/http/api/client"
	"github.com/b3scale/b3scale/pkg/store"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thcyron/skop/v2/reconcile"
	"github.com/thcyron/skop/v2/skop"
	corev1 "k8s.io/api/core/v1"
	kubernetesErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var FINALIZER_URL = "b3scale.infra.run/finalizer"

type B3ScaleOperator struct {
	logger    log.Logger
	op        *skop.Operator
	apiClient *b3scaleclient.Client
	config    *config2.Config
}

func NewB3ScaleOperator(config *config2.Config) (*B3ScaleOperator, error) {
	logger := makeLogger()

	kubernetesConfig, err := GetKubernetesConfig(&config.Kubernetes)
	if err != nil {
		return nil, err
	}

	apiUrl := fmt.Sprintf("https://%v", config.B3Scale.Host)
	apiClient := b3scaleclient.New(
		apiUrl,
		config.B3Scale.AccessToken,
	)

	b3ScaleOperator := B3ScaleOperator{
		logger:    logger,
		apiClient: apiClient,
		config:    config,
	}

	op := skop.New(
		skop.WithResource("b3scale.infra.run", "v1", "bbbfrontends", &v1.BBBFrontend{}),
		skop.WithConfig(kubernetesConfig),
		skop.WithReconciler(&b3ScaleOperator),
		skop.WithLogger(logger),
	)

	b3ScaleOperator.op = op

	return &b3ScaleOperator, nil

}

func (o *B3ScaleOperator) Run() error {
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- o.op.Run()
	}()

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		level.Info(o.logger).Log(
			"msg", "Received SIGINT or SIGTERM",
			"signal", sig,
		)
		o.op.Stop()
		return nil
	case err := <-runErrCh:
		level.Error(o.logger).Log(
			"msg", "Operator errored, exiting...",
			"err", err,
		)
		return err
	}
}

func (o *B3ScaleOperator) Reconcile(ctx context.Context, op *skop.Operator, res skop.Resource) error {
	operatorKubernetesClient := NewOperatorKubernetesClient(op.Clientset())
	bbbFrontend := res.(*v1.BBBFrontend)

	reconcileError := o.innerReconcile(ctx, op, bbbFrontend)

	if bbbFrontend.DeletionTimestamp != nil {
		// We do not update the status, if we are currently processing a deletion
		return reconcileError
	}

	if reconcileError != nil {
		// This should be the latest version of this resource anyway.
		// We always pass a reference to anywhere.
		err := operatorKubernetesClient.SetReadyStatusCondition(ctx, bbbFrontend, false, reconcileError)
		if err != nil {
			return err
		} else {
			return reconcileError
		}
	} else {
		err := operatorKubernetesClient.SetReadyStatusCondition(ctx, bbbFrontend, true, nil)
		return err
	}

}

func (o *B3ScaleOperator) innerReconcile(ctx context.Context, op *skop.Operator, bbbFrontend *v1.BBBFrontend) error {

	operatorKubernetesClient := NewOperatorKubernetesClient(op.Clientset())

	uniqName := fmt.Sprintf("b3o-%v-%v", bbbFrontend.ObjectMeta.Namespace, bbbFrontend.ObjectMeta.Name)

	cM, configMapError := op.Clientset().CoreV1().ConfigMaps(bbbFrontend.ObjectMeta.Namespace).Get(ctx, uniqName, metav1.GetOptions{})
	if configMapError != nil && !kubernetesErrors.IsNotFound(configMapError) {
		return configMapError
	}

	configMap := cM.DeepCopy()

	if bbbFrontend.DeletionTimestamp != nil {
		// Deletion

		// Check, if we need to remove the finalizers.

		var existingFrontend *store.FrontendState
		// If the configMap is already deleted, everything is fine or we screwed something up.
		if configMap != nil {
			frontendId, ok := configMap.Data["FRONTEND_ID"]
			if !ok {
				level.Debug(o.logger).Log(
					"msg", "Could not find FRONTEND_ID in ConfigMap",
					"configMap", configMap,
					"cM", cM,
					"configMapError", configMapError,
				)
				return errors.New("Invalid configMap, FRONTEND_ID not found")
			}

			x, err := o.apiClient.FrontendRetrieve(ctx, frontendId)
			existingFrontend = x

			if err != nil {
				return err
			}
		}

		if existingFrontend != nil {
			_, err := o.apiClient.FrontendDelete(ctx, existingFrontend)

			if err != nil {
				return err
			}

		}

		err := operatorKubernetesClient.RemoveFinalizerFromConfigMap(ctx, configMap, FINALIZER_URL)
		if err != nil {
			return err
		}

		err = operatorKubernetesClient.RemoveFinalizerFromBBBFrontend(ctx, bbbFrontend, FINALIZER_URL)
		if err != nil {
			return err
		}

		return nil
	}

	secret, secretError := op.Clientset().CoreV1().Secrets(bbbFrontend.ObjectMeta.Namespace).Get(ctx, uniqName, metav1.GetOptions{})
	if secretError != nil && !kubernetesErrors.IsNotFound(secretError) {
		return secretError
	}

	var userConfiguredSecret *corev1.Secret
	if bbbFrontend.Spec.Credentials != nil {
		uSecret, userSecretError := op.Clientset().CoreV1().Secrets(bbbFrontend.ObjectMeta.Namespace).Get(ctx, bbbFrontend.Spec.Credentials.SecretRef.Name, metav1.GetOptions{})
		if userSecretError != nil {
			return userSecretError
		}

		userConfiguredSecret = uSecret
	}

	var frontendSecret string

	// Prepopulating Secret, if we do not have one and there is no other secret configured.
	if secretError != nil && userConfiguredSecret == nil {
		generatedSecret := util.GenerateSecureToken(21)

		newSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uniqName,
				Namespace: bbbFrontend.ObjectMeta.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind:       bbbFrontend.Kind,
						APIVersion: bbbFrontend.APIVersion,
						Name:       bbbFrontend.ObjectMeta.Name,
						UID:        bbbFrontend.ObjectMeta.UID,
						Controller: skop.Bool(true),
					},
				},
			},
			StringData: map[string]string{
				"FRONTEND_SECRET": generatedSecret,
			},
		}

		err := reconcile2.Secret(ctx, o.op.Clientset(), &newSecret)
		if err != nil {
			return err
		}

		frontendSecret = generatedSecret

	} else if userConfiguredSecret != nil {
		t, ok := userConfiguredSecret.Data[bbbFrontend.Spec.Credentials.SecretRef.Key]
		if !ok {
			return errors.New("invalid secret or wrong key given, did not find existing secret")
		}

		tStr := string(t)

		if len(tStr) < 32 {
			return errors.New("secret is too short, cannot be used`")
		}

		frontendSecret = tStr
	} else {
		frontendSecret = string(secret.Data["FRONTEND_SECRET"])
	}

	var frontendKey string
	if bbbFrontend.Spec.Credentials != nil {
		frontendKey = bbbFrontend.Spec.Credentials.Key
	} else {
		frontendKey = uniqName
	}

	if configMapError != nil {
		// Create ConfigMap and Secrets and so on and create resource in B3Scale backend

		// FIXME: Use FrontendCreateRaw to be sane with the update functionality. Technically this has different behaviour.
		createdFrontend, err := o.apiClient.FrontendCreate(ctx, &store.FrontendState{
			Active: true,
			Frontend: &bbb.Frontend{
				Key:    frontendKey,
				Secret: frontendSecret,
			},
			Settings: bbbFrontend.Spec.Settings.ToAPIFrontendSettings(),
		})

		if err != nil {
			return err
		}

		err = operatorKubernetesClient.AddFinalizerToBBBFrontend(ctx, bbbFrontend, FINALIZER_URL)
		if err != nil {
			return err
		}

		newConfigMap := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uniqName,
				Namespace: bbbFrontend.ObjectMeta.Namespace,
				Finalizers: []string{
					FINALIZER_URL,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind:       bbbFrontend.Kind,
						APIVersion: bbbFrontend.APIVersion,
						Name:       bbbFrontend.ObjectMeta.Name,
						UID:        bbbFrontend.ObjectMeta.UID,
						Controller: skop.Bool(true),
					},
				},
			},
			Data: map[string]string{
				"FRONTEND_ENDPOINT": fmt.Sprintf("https://%v/bbb/%v", o.config.B3Scale.Host, createdFrontend.Frontend.Key),
				"FRONTEND_ID":       createdFrontend.ID,
			},
		}

		err = reconcile.ConfigMap(ctx, o.op.Clientset(), &newConfigMap)
		if err != nil {
			return err
		}

	} else {
		// Existing configMap and Secret, reusing it.
		frontendId, ok := configMap.Data["FRONTEND_ID"]
		if !ok {
			return errors.New("Invalid configMap, FRONTEND_ID not found")
		}

		existingFrontend, err := o.apiClient.FrontendRetrieve(ctx, frontendId)

		if err != nil {
			return err
		}

		cleanedSettings := util.GetCleanedFrontendSettings(&bbbFrontend.Spec.Settings)

		payload, err := json.Marshal(
			map[string]v1.FrontendSettings{
				"settings": cleanedSettings,
			},
		)

		_, err = o.apiClient.FrontendUpdateRaw(ctx, existingFrontend.ID, payload)

		if err != nil {
			return err
		}
	}

	return nil
}

func makeLogger() log.Logger {
	var logger log.Logger
	logger = log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	return logger
}

func makeConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
