package v1

import (
	"github.com/b3scale/b3scale/pkg/bbb"
	"github.com/b3scale/b3scale/pkg/store"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FrontendSettings mirrors store.FrontendSettings but uses camelCase instead of snake_case to better reflect
// what is commonly used in the Kubernetes world.
type FrontendSettings struct {
	RequiredTags         []string                           `json:"requiredTags"`
	DefaultPresentation  *store.DefaultPresentationSettings `json:"defaultPresentation"`
	CreateDefaultParams  *bbb.Params                        `json:"createDefaultParams"`
	CreateOverrideParams *bbb.Params                        `json:"createOverrideParams"`
}

type Credentials struct {
	Frontend  string               `json:"frontend"`
	SecretRef v1.SecretKeySelector `json:"secretRef"`
}

type BBBFrontend struct {
	metav1.ObjectMeta `json:"metadata"`
	Kind              string            `json:"kind"`
	APIVersion        string            `json:"apiVersion"`
	Spec              BBBFrontendSpecs  `json:"spec"`
	Status            BBBFrontendStatus `json:"status"`
}

type BBBFrontendStatus struct {
	Conditions []metav1.Condition `json:"conditions"`
}

type BBBFrontendSpecs struct {
	Settings           FrontendSettings `json:"settings"`
	Credentials        *Credentials     `json:"credentials"`
	DeletionProtection bool             `json:"deletionProtection"`
	FrontendID         *string          `json:"frontendID"`
}

func (f *FrontendSettings) ToAPIFrontendSettings() store.FrontendSettings {
	var defaultPresentation *store.DefaultPresentationSettings
	if f.DefaultPresentation != nil {
		defaultPresentation = &store.DefaultPresentationSettings{
			URL:   f.DefaultPresentation.URL,
			Force: f.DefaultPresentation.Force,
		}
	}

	requiredTags := make([]string, 0)
	if f.RequiredTags != nil {
		requiredTags = f.RequiredTags
	}

	createDefaultParams := make(bbb.Params)
	if f.CreateDefaultParams != nil && len(*f.CreateDefaultParams) > 0 {
		createDefaultParams = *f.CreateDefaultParams
	}

	createOverrideParams := make(bbb.Params)
	if f.CreateOverrideParams != nil && len(*f.CreateOverrideParams) > 0 {
		createOverrideParams = *f.CreateOverrideParams
	}

	s := store.FrontendSettings{
		RequiredTags:         requiredTags,
		DefaultPresentation:  defaultPresentation,
		CreateDefaultParams:  createDefaultParams,
		CreateOverrideParams: createOverrideParams,
	}

	return s
}
