package server

import (
  "encoding/json"
  "net/http"
  "strconv"

  "github.com/golang/glog"
  "github.com/gorilla/mux"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  "k8s.io/apimachinery/pkg/labels"
  "k8s.io/client-go/tools/cache"

  vmapi "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
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

func (s *server) InstanceCreate(w http.ResponseWriter, r *http.Request) {
  r.ParseForm()
  // TODO validate form
  ns := r.PostForm["ns"][0]
  name := r.PostForm["name"][0]
  cpus, _ := strconv.Atoi(r.PostForm["cpus"][0])
  mem, _ := strconv.Atoi(r.PostForm["mem"][0])
  image := vmapi.MachineImageType(r.PostForm["image"][0])
  action := vmapi.ActionType(r.PostForm["action"][0])

  vm := &vmapi.VirtualMachine{
    ObjectMeta: metav1.ObjectMeta{
      Name:      name,
    },
    Spec: vmapi.VirtualMachineSpec{
      Cpus: int32(cpus),
      MemoryMB: int32(mem),
      MachineImage: image,
      Action: action,
    },
  }

  vm, err := s.vmClient.VirtualmachineV1alpha1().VirtualMachines(ns).Create(vm)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
  } else {
    w.WriteHeader(http.StatusCreated)
  }
}

func (s *server) InstanceDelete(w http.ResponseWriter, r *http.Request) {
  ns := mux.Vars(r)["ns"]
  name := mux.Vars(r)["name"]

  err := s.vmClient.VirtualmachineV1alpha1().VirtualMachines(ns).Delete(name, &metav1.DeleteOptions{})
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    w.Write([]byte(err.Error()))
  } else {
    w.WriteHeader(http.StatusNoContent)
  }
}
