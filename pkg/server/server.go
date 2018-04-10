package server

import (
	"net/http"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	vmclientset "github.com/llparse/kube-crd-skel/pkg/client/clientset/versioned"
	vminformers "github.com/llparse/kube-crd-skel/pkg/client/informers/externalversions/virtualmachine/v1alpha1"
	vmlisters "github.com/llparse/kube-crd-skel/pkg/client/listers/virtualmachine/v1alpha1"
)

type server struct {
	vmClient   vmclientset.Interface
	kubeClient kubernetes.Interface

	vmLister         vmlisters.VirtualMachineLister
	vmListerSynced   cache.InformerSynced
	nodeLister       corelisters.NodeLister
	nodeListerSynced cache.InformerSynced
	credLister       vmlisters.CredentialLister
	credListerSynced cache.InformerSynced
}

func NewServer(
	vmClient vmclientset.Interface,
	kubeClient kubernetes.Interface,
	vmInformer vminformers.VirtualMachineInformer,
	nodeInformer coreinformers.NodeInformer,
	credInformer vminformers.CredentialInformer,
) *server {

	return &server{
		vmClient:   vmClient,
		kubeClient: kubeClient,

		vmLister:         vmInformer.Lister(),
		vmListerSynced:   vmInformer.Informer().HasSynced,
		nodeLister:       nodeInformer.Lister(),
		nodeListerSynced: nodeInformer.Informer().HasSynced,
		credLister:       credInformer.Lister(),
		credListerSynced: credInformer.Informer().HasSynced,
	}
}

func (s *server) Run(stopCh <-chan struct{}) {
	if !cache.WaitForCacheSync(stopCh, s.vmListerSynced, s.nodeListerSynced, s.credListerSynced) {
		return
	}

	r := s.newRouter()
	glog.Info("Starting http server listening on :9500")
	go http.ListenAndServe(":9500", r)

	<-stopCh
}

func (s *server) newRouter() *mux.Router {
	r := mux.NewRouter().StrictSlash(true)

	r.Methods("GET").Path("/v1/instances").Handler(http.HandlerFunc(s.InstanceList))
	r.Methods("POST").Path("/v1/instances").Handler(http.HandlerFunc(s.InstanceCreate))
	r.Methods("DELETE").Path("/v1/instances/{ns}/{name}").Handler(http.HandlerFunc(s.InstanceDelete))
	r.Methods("POST").Path("/v1/instances/{ns}/{name}/{action}").Handler(http.HandlerFunc(s.InstanceAction))
	r.Methods("GET").Path("/v1/host").Handler(http.HandlerFunc(s.NodeList))

	r.Methods("GET").Path("/v1/credential").Handler(http.HandlerFunc(s.CredentialList))
	r.Methods("DELETE").Path("/v1/credential/{name}").Handler(http.HandlerFunc(s.CredentialDelete))
	return r
}
