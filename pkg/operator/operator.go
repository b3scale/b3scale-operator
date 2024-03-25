package operator

import (
	"context"
	"errors"
	"k8s.io/apimachinery/pkg/util/json"

	"fmt"
	"os"
	"os/signal"
	"syscall"

	v1 "github.com/b3scale/b3scale-operator/pkg/apis/v1"
	"github.com/b3scale/b3scale-operator/pkg/config"
	"github.com/b3scale/b3scale/pkg/bbb"
	b3scaleclient "github.com/b3scale/b3scale/pkg/http/api/client"
	"github.com/b3scale/b3scale/pkg/store"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thcyron/skop/v2/skop"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var FINALIZER_URL = "b3scale.io/finalizer"

type B3ScaleOperator struct {
	logger    log.Logger
	op        *skop.Operator
	apiClient *b3scaleclient.Client
	config    *config.Config
}

func NewB3ScaleOperator(config *config.Config) (*B3ScaleOperator, error) {
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
		skop.WithResource("b3scale.io", "v1", "bbbfrontends", &v1.BBBFrontend{}),
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

	// Deletion
	if bbbFrontend.DeletionTimestamp != nil {
		// Check if we need to remove the finalizers
		if bbbFrontend.Spec.FrontendID != nil {
			existingFrontend, err := o.apiClient.FrontendRetrieve(ctx, *bbbFrontend.Spec.FrontendID)
			if err != nil {
				return err
			}

			if existingFrontend != nil && !bbbFrontend.Spec.DeletionProtection {
				_, err := o.apiClient.FrontendDelete(ctx, existingFrontend)
				if err != nil {
					return err
				}
			}
		}

		err := operatorKubernetesClient.RemoveFinalizerFromBBBFrontend(ctx, bbbFrontend, FINALIZER_URL)
		if err != nil {
			return err
		}

		return nil
	}

	// Validation
	if bbbFrontend.Spec.Credentials == nil {
		return errors.New(fmt.Sprintf("BBBFrontend [%s/%s] has no credentials configured",
			bbbFrontend.Namespace, bbbFrontend.Name))
	}
	if len(bbbFrontend.Spec.Credentials.Frontend) == 0 {
		return errors.New(fmt.Sprintf("BBBFrontend [%s/%s] has no frontend configured",
			bbbFrontend.Namespace, bbbFrontend.Name))
	}
	frontendSecret, err := extractFrontendSecret(ctx, op, bbbFrontend)
	if err != nil {
		return err
	}

	if bbbFrontend.Spec.FrontendID == nil {
		// Create frontend in B3Scale backend
		createdFrontend, err := o.apiClient.FrontendCreate(ctx, &store.FrontendState{
			Active: true,
			Frontend: &bbb.Frontend{
				Key:    bbbFrontend.Spec.Credentials.Frontend,
				Secret: frontendSecret,
			},
			Settings: bbbFrontend.Spec.Settings.ToAPIFrontendSettings(),
		})
		if err != nil {
			return err
		}

		err = operatorKubernetesClient.CompleteBBBFrontend(ctx, bbbFrontend, FINALIZER_URL, createdFrontend.ID)
		if err != nil {
			return err
		}
	} else {
		// Update frontend in B3Scale backend
		payload, err := json.Marshal(
			map[string]store.FrontendSettings{
				"settings": bbbFrontend.Spec.Settings.ToAPIFrontendSettings(),
			},
		)
		if err != nil {
			return err
		}

		_, err = o.apiClient.FrontendUpdateRaw(ctx, *bbbFrontend.Spec.FrontendID, payload)
		if err != nil {
			return err
		}
	}

	return nil
}

func extractFrontendSecret(ctx context.Context, op *skop.Operator, bbbFrontend *v1.BBBFrontend) (string, error) {
	secret, err := op.Clientset().CoreV1().Secrets(bbbFrontend.ObjectMeta.Namespace).Get(ctx, bbbFrontend.Spec.Credentials.SecretRef.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	secretKey, ok := secret.Data[bbbFrontend.Spec.Credentials.SecretRef.Key]
	if !ok {
		return "", errors.New("invalid secret or wrong key given, did not find existing secret")
	}
	frontendSecret := string(secretKey)
	if len(frontendSecret) < 32 {
		return "", errors.New("secret is too short, cannot be used`")
	}
	return frontendSecret, nil
}

func makeLogger() log.Logger {
	var logger log.Logger
	logger = log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	return logger
}
