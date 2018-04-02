package server

import (
  "encoding/json"
  "net/http"

  "github.com/golang/glog"
  "github.com/gorilla/mux"
  "k8s.io/apimachinery/pkg/labels"
  "k8s.io/client-go/tools/cache"

  vmapi "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
  vminformers "github.com/llparse/kube-crd-skel/pkg/client/informers/externalversions/virtualmachine/v1alpha1"
  vmlisters "github.com/llparse/kube-crd-skel/pkg/client/listers/virtualmachine/v1alpha1"
)

type server struct {
  vmLister        vmlisters.VirtualMachineLister
  vmListerSynced  cache.InformerSynced
}

func NewServer(vmInformer vminformers.VirtualMachineInformer) *server {
  return &server{
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

  // r.Methods("GET").Path("/").Handler(versionsHandler)
  // r.Methods("GET").Path("/version").Handler(versionHandler)
  r.Methods("GET").Path("/v1/instances").Handler(http.HandlerFunc(s.InstanceList))
  return r
}

type InstanceList struct {
  Instances []*vmapi.VirtualMachine `json:"data"`
}

func (s *server) InstanceList(w http.ResponseWriter, r *http.Request) {
  vms, err := s.vmLister.List(labels.Everything())
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  resp, err := json.Marshal(InstanceList{
    Instances: vms,
  })

  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  w.Header().Set("Content-Type", "application/json")
  w.Write(resp)
}
