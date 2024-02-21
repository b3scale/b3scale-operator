package util

import (
	"crypto/rand"
	"encoding/hex"

	v1 "github.com/b3scale/b3scale-operator/pkg/apis/v1"
	"github.com/b3scale/b3scale/pkg/bbb"
)

func GenerateSecureToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func GetCleanedFrontendSettings(f *v1.FrontendSettings) v1.FrontendSettings {

	var defaultPresentation *v1.DefaultPresentationSettings
	if f.DefaultPresentation != nil {
		defaultPresentation = &v1.DefaultPresentationSettings{
			URL:   f.DefaultPresentation.URL,
			Force: f.DefaultPresentation.Force,
		}
	}

	requiredTags := make([]string, 0)
	if f.RequiredTags != nil {
		requiredTags = f.RequiredTags
	}

	var createDefaultParams *bbb.Params
	if f.CreateDefaultParams != nil && len(*f.CreateDefaultParams) > 0 {
		createDefaultParams = f.CreateDefaultParams
	}

	var createOverrideParams *bbb.Params
	if f.CreateOverrideParams != nil && len(*f.CreateOverrideParams) > 0 {
		createOverrideParams = f.CreateOverrideParams
	}

	return v1.FrontendSettings{
		RequiredTags:         requiredTags,
		DefaultPresentation:  defaultPresentation,
		CreateDefaultParams:  createDefaultParams,
		CreateOverrideParams: createOverrideParams,
	}
}
