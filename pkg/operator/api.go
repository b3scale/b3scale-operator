package operator

import (
	"context"
	"fmt"
	v1 "gitlab.com/infra.run/public/b3scale-operator/pkg/apis/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
)

func updateBBBFrontend(ctx context.Context, cs *kubernetes.Clientset, bbb *v1.BBBFrontend) error {
	jsonBody, err := json.Marshal(bbb)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("/apis/b3scale.infra.run/v1/namespaces/%v/bbbfrontends/%v", bbb.Namespace, bbb.Name)
	d := cs.RESTClient().Put().AbsPath(url).Body(jsonBody).Do(ctx)
	if d.Error() != nil {
		return d.Error()
	}

	return nil
}
