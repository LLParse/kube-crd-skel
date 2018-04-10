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
	"github.com/llparse/kube-crd-skel/pkg/controller/ip"
	"github.com/llparse/kube-crd-skel/pkg/controller/vm"
	"github.com/llparse/kube-crd-skel/pkg/server"
)

func main() {
	vmCtrl := flag.Bool("vm", false, "Run the VM controller")
	ipCtrl := flag.Bool("ip", false, "Run the IP controller")
	nodeName := flag.String("nodename", "", "Name of the node running the controller pod")
	serv := flag.Bool("server", false, "Run the rest server")

	kubeconfig := flag.String("kubeconfig", "", "Path to a kube config; only required if out-of-cluster.")
	workers := flag.Int("workers", 5, "Concurrent VM syncs")
	flag.Set("logtostderr", "true")
	flag.Parse()

	config, err := NewKubeClientConfig(*kubeconfig)
	if err != nil {
		panic(err)
	}

	apiextensionsclientset := apiextensionsclient.NewForConfigOrDie(config)
	if err := ranchervm.CreateVirtualMachineDefinition(apiextensionsclientset); err != nil && !apierrors.IsAlreadyExists(err) {
		panic(err)
	}
	if err := ranchervm.CreateARPTableDefinition(apiextensionsclientset); err != nil && !apierrors.IsAlreadyExists(err) {
		panic(err)
	}
	if err := ranchervm.CreateCredentialDefinition(apiextensionsclientset); err != nil && !apierrors.IsAlreadyExists(err) {
		panic(err)
	}

	vmClientset := versioned.NewForConfigOrDie(config)
	vmInformerFactory := externalversions.NewSharedInformerFactory(vmClientset, 0*time.Second)

	kubeClientset := kubernetes.NewForConfigOrDie(config)
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClientset, 0*time.Second)

	stopCh := makeStopChan()

	if *vmCtrl {
		go vm.NewVirtualMachineController(
			vmClientset,
			kubeClientset,
			vmInformerFactory.Virtualmachine().V1alpha1().VirtualMachines(),
			kubeInformerFactory.Core().V1().Pods(),
			kubeInformerFactory.Core().V1().Services(),
			vmInformerFactory.Virtualmachine().V1alpha1().Credentials(),
		).Run(*workers, stopCh)
	}

	if *ipCtrl {
		go ip.NewIPDiscoveryController(
			vmClientset,
			vmInformerFactory.Virtualmachine().V1alpha1().ARPTables(),
			vmInformerFactory.Virtualmachine().V1alpha1().VirtualMachines(),
			kubeInformerFactory.Core().V1().Namespaces(),
			*nodeName,
		).Run(*workers, stopCh)
	}

	if *serv {
		go server.NewServer(
			vmClientset,
			kubeClientset,
			vmInformerFactory.Virtualmachine().V1alpha1().VirtualMachines(),
			kubeInformerFactory.Core().V1().Nodes(),
			vmInformerFactory.Virtualmachine().V1alpha1().Credentials(),
		).Run(stopCh)
	}

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
