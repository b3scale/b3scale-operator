package v1

import (
	"gitlab.com/infra.run/public/b3scale/pkg/store"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DefaultPresentationSettings struct {
	URL   string `json:"url"`
	Force bool   `json:"force"`
}

type FrontendSettings struct {
	RequiredTags        []string                     `json:"required_tags"`
	DefaultPresentation *DefaultPresentationSettings `json:"default_presentation"`
}

type BBBFrontend struct {
	metav1.ObjectMeta `json:"metadata"`
	Kind              string           `json:"kind"`
	APIVersion        string           `json:"apiVersion"`
	Spec              BBBFrontendSpecs `json:"spec"`
}

type BBBFrontendSpecs struct {
	Settings FrontendSettings `json:"settings"`
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

	s := store.FrontendSettings{
		RequiredTags:        requiredTags,
		DefaultPresentation: defaultPresentation,
	}

	return s
}
