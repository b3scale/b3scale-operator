package operator

import (
	"errors"
	config2 "gitlab.com/infra.run/public/b3scale-operator/pkg/config"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetKubernetesConfig(config *config2.Kubernetes) (*rest.Config, error) {

	if config.InCluster != nil {
		return rest.InClusterConfig()
	} else if config.OutOfCluster != nil {
		return clientcmd.BuildConfigFromFlags("", config.OutOfCluster.FileName)
	} else {
		return nil, errors.New("invalid kubernetes configuration section, needs InCluster or OutOfCluster authentication")
	}
}
