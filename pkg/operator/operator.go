package operator

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/thcyron/skop/v2/reconcile"
	"github.com/thcyron/skop/v2/skop"
	v1 "gitlab.com/infra.run/public/b3scale-operator/pkg/apis/v1"
	config2 "gitlab.com/infra.run/public/b3scale-operator/pkg/config"
	reconcile2 "gitlab.com/infra.run/public/b3scale-operator/pkg/reconcile"
	"gitlab.com/infra.run/public/b3scale-operator/pkg/util"
	"gitlab.com/infra.run/public/b3scale/pkg/bbb"
	b3scalehttpv1 "gitlab.com/infra.run/public/b3scale/pkg/http/api/v1"
	"gitlab.com/infra.run/public/b3scale/pkg/store"
	corev1 "k8s.io/api/core/v1"
	kubernetesErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"os/signal"
	"syscall"
)

var FINALIZER_URL = "b3scale.infra.run"

type B3ScaleOperator struct {
	logger    log.Logger
	op        *skop.Operator
	apiClient b3scalehttpv1.Client
	config    *config2.Config
}

func NewB3ScaleOperator(config *config2.Config) (*B3ScaleOperator, error) {
	logger := makeLogger()

	kubernetesConfig, err := GetKubernetesConfig(&config.Kubernetes)
	if err != nil {
		return nil, err
	}

	apiClient := b3scalehttpv1.NewJWTClient(
		config.B3Scale.Host,
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

	bbbFrontend := res.(*v1.BBBFrontend)
	uniqName := fmt.Sprintf("b3scale-operator-%v", bbbFrontend.Name)

	configMap, err := op.Clientset().CoreV1().ConfigMaps(bbbFrontend.Namespace).Get(ctx, uniqName, metav1.GetOptions{})

	if err != nil && !kubernetesErrors.IsNotFound(err) {
		return err
	}

	if err != nil {
		// Create ConfigMap and Secrets and so on and create resource in B3Scale backend

		secret := util.GenerateSecureToken(21)

		createdFrontend, err := o.apiClient.FrontendCreate(ctx, &store.FrontendState{
			Active: true,
			Frontend: &bbb.Frontend{
				Key:    uniqName,
				Secret: secret,
			},
			Settings: bbbFrontend.Spec.Settings.ToAPIFrontendSettings(),
		})

		if err != nil {
			return err
		}

		bbbFrontend.SetFinalizers([]string{FINALIZER_URL})
		err = updateBBBFrontend(ctx, o.op.Clientset(), bbbFrontend)
		if err != nil {
			return err
		}

		newConfigMap := corev1.ConfigMap{
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
			Data: map[string]string{
				"FRONTEND_HOST": o.config.B3Scale.Host,
				"FRONTEND_ID":   createdFrontend.ID,
			},
		}

		err = reconcile.ConfigMap(ctx, o.op.Clientset(), &newConfigMap)
		if err != nil {
			return err
		}

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
				"FRONTEND_KEY":    createdFrontend.Frontend.Key,
				"FRONTEND_SECRET": createdFrontend.Frontend.Secret,
			},
		}

		err = reconcile2.Secret(ctx, o.op.Clientset(), &newSecret)
		if err != nil {
			return err
		}

	} else if bbbFrontend.DeletionTimestamp != nil {
		// Deletion

		frontendId, ok := configMap.Data["FRONTEND_ID"]
		if !ok {
			return errors.New("Invalid configMap, FRONTEND_ID not found")
		}

		existingFrontend, err := o.apiClient.FrontendRetrieve(ctx, frontendId)

		if err != nil {
			return err
		}

		_, err = o.apiClient.FrontendDelete(ctx, existingFrontend)

		if err != nil {
			return err
		}

		// Remove Finalizer, so Kubernetes can safely remove this object.

		// We want to remove our finalizer, but keep the other finalizers.
		finalizers := []string{}
		for _, finalizer := range bbbFrontend.Finalizers {
			if finalizer != FINALIZER_URL {
				finalizers = append(finalizers, finalizer)
			}
		}

		bbbFrontend.SetFinalizers(finalizers)
		err = updateBBBFrontend(ctx, o.op.Clientset(), bbbFrontend)
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

		existingFrontend.Settings = bbbFrontend.Spec.Settings.ToAPIFrontendSettings()
		_, err = o.apiClient.FrontendUpdate(ctx, existingFrontend)

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
