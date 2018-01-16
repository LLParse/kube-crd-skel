package main

import (
	"flag"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/llparse/kube-crd-skel/pkg/apis/virtualmachine"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "Path to a kube config; only required if out-of-cluster.")
	flag.Parse()

	config, err := NewKubeClientConfig(*kubeconfig)
	if err != nil {
		panic(err)
	}

	clientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	if err := virtualmachine.CreateCustomResourceDefinition(clientset); err != nil {
		panic(err)
	}
}

func NewKubeClientConfig(configPath string) (*rest.Config, error) {
	if configPath != "" {
		return clientcmd.BuildConfigFromFlags("", configPath)
	}
	return rest.InClusterConfig()
}
