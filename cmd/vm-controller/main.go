package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/glog"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/llparse/kube-crd-skel/pkg/apis/ranchervm"
	"github.com/llparse/kube-crd-skel/pkg/client/clientset/versioned"
	"github.com/llparse/kube-crd-skel/pkg/client/informers/externalversions"
	"github.com/llparse/kube-crd-skel/pkg/controller/vm"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "Path to a kube config; only required if out-of-cluster.")
	workers := flag.Int("workers", 5, "Concurrent VM syncs")
	flag.Set("logtostderr", "true")
	flag.Parse()

	config, err := NewKubeClientConfig(*kubeconfig)
	if err != nil {
		panic(err)
	}

	apiextensionsclientset := apiextensionsclient.NewForConfigOrDie(config)
	if err := ranchervm.CreateCustomResourceDefinition(apiextensionsclientset); err != nil && !apierrors.IsAlreadyExists(err) {
		panic(err)
	}

	vmClientset := versioned.NewForConfigOrDie(config)
	vmInformerFactory := externalversions.NewSharedInformerFactory(vmClientset, 0*time.Second)

	kubeClientset := kubernetes.NewForConfigOrDie(config)
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClientset, 0*time.Second)

	stopCh := makeStopChan()

	go vm.NewVirtualMachineController(
		vmClientset,
		kubeClientset,
		vmInformerFactory.Virtualmachine().V1alpha1().VirtualMachines(),
		kubeInformerFactory.Core().V1().Pods(),
		kubeInformerFactory.Core().V1().Services(),
	).Run(*workers, stopCh)

	vmInformerFactory.Start(stopCh)
	kubeInformerFactory.Start(stopCh)

	<-stopCh
}

func NewKubeClientConfig(configPath string) (*rest.Config, error) {
	if configPath != "" {
		return clientcmd.BuildConfigFromFlags("", configPath)
	}
	return rest.InClusterConfig()
}

func makeStopChan() <-chan struct{} {
	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-c
		glog.Info("Received stop signal, attempting graceful termination...")
		close(stop)
		<-c
		glog.Info("Received stop signal, terminating immediately!")
		os.Exit(1)
	}()
	return stop
}
