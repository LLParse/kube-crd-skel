package server

import (
  "net/http"

  "github.com/golang/glog"
  "github.com/gorilla/mux"
  "k8s.io/client-go/tools/cache"

  vmclientset "github.com/llparse/kube-crd-skel/pkg/client/clientset/versioned"
  vminformers "github.com/llparse/kube-crd-skel/pkg/client/informers/externalversions/virtualmachine/v1alpha1"
  vmlisters "github.com/llparse/kube-crd-skel/pkg/client/listers/virtualmachine/v1alpha1"
)

type server struct {
  vmClient   vmclientset.Interface
  vmLister        vmlisters.VirtualMachineLister
  vmListerSynced  cache.InformerSynced
}

func NewServer(vmClient vmclientset.Interface, vmInformer vminformers.VirtualMachineInformer) *server {
  return &server{
    vmClient: vmClient,
    vmLister: vmInformer.Lister(),
    vmListerSynced: vmInformer.Informer().HasSynced,
  }
}

func (s *server) Run(stopCh <-chan struct{}) {
  // wait for cache to sync before opening server
  if !cache.WaitForCacheSync(stopCh, s.vmListerSynced) {
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
  return r
}
