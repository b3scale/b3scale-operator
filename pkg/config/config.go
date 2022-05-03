package config

type KubernetesOutOfCluster struct {
	FileName string `json:"fileName"`
}

type KubernetesInCluster struct{}

type Kubernetes struct {
	OutOfCluster *KubernetesOutOfCluster `json:"outOfCluster"`
	InCluster    *KubernetesInCluster    `json:"inCluster"`
}

type B3Scale struct {
	Host        string `json:"host"`
	AccessToken string `json:"accessToken"`
}

type Config struct {
	Kubernetes Kubernetes `json:"kubernetes"`
	B3Scale    B3Scale    `json:"b3Scale"`
}
