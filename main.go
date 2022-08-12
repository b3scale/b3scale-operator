package main

import (
	config2 "github.com/b3scale/b3scale-operator/pkg/config"
	operator2 "github.com/b3scale/b3scale-operator/pkg/operator"
	"github.com/spf13/viper"
)

func startUpOperator() error {

	viper.SetConfigFile("b3scale-operator-config.yaml")
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}

	var config config2.Config
	err = viper.Unmarshal(&config)

	if err != nil {
		return err
	}

	operator, err := operator2.NewB3ScaleOperator(&config)

	if err != nil {
		return err
	}

	return operator.Run()

}

func main() {

	err := startUpOperator()
	if err != nil {
		panic(err)
	}
}
