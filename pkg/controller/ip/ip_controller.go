package ip

import (
  "bufio"
  "os"
  "strings"
  "time"

  "github.com/golang/glog"
  apierrors "k8s.io/apimachinery/pkg/api/errors"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  "k8s.io/apimachinery/pkg/util/wait"
  "k8s.io/client-go/tools/cache"
  "k8s.io/client-go/util/workqueue"

  vmapi "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
  vmclientset "github.com/llparse/kube-crd-skel/pkg/client/clientset/versioned"
  vminformers "github.com/llparse/kube-crd-skel/pkg/client/informers/externalversions/virtualmachine/v1alpha1"
  vmlisters "github.com/llparse/kube-crd-skel/pkg/client/listers/virtualmachine/v1alpha1"
)

type IPDiscoveryController struct {
  crdClient   vmclientset.Interface

  arpLister        vmlisters.ARPTableLister
  arpListerSynced  cache.InformerSynced

  arpQueue  workqueue.RateLimitingInterface

  nodeName string
}

func NewIPDiscoveryController(
  crdClient vmclientset.Interface,
  arpInformer vminformers.ARPTableInformer,
  nodeName string,
) *IPDiscoveryController {

  ctrl := &IPDiscoveryController{
    crdClient:  crdClient,
    arpQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "virtualmachine"),
    nodeName: nodeName,
  }

  arpInformer.Informer().AddEventHandler(
    cache.ResourceEventHandlerFuncs{
      AddFunc:    func(obj interface{}) { ctrl.enqueueWork(ctrl.arpQueue, obj) },
      UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueWork(ctrl.arpQueue, newObj) },
      DeleteFunc: func(obj interface{}) { ctrl.enqueueWork(ctrl.arpQueue, obj) },
    },
  )

  ctrl.arpLister = arpInformer.Lister()
  ctrl.arpListerSynced = arpInformer.Informer().HasSynced

  return ctrl
}

func (ctrl *IPDiscoveryController) Run(workers int, stopCh <-chan struct{}) {
  defer ctrl.arpQueue.ShutDown()

  glog.Infof("Starting ip discovery controller")
  defer glog.Infof("Shutting down ip discovery Controller")

  if !cache.WaitForCacheSync(stopCh, ctrl.arpListerSynced) {
    return
  }

  go wait.Until(ctrl.arpWorker, time.Second, stopCh)
  go ctrl.updatePeriodically()

  <-stopCh
}

func (ctrl *IPDiscoveryController) updatePeriodically() {
  c := time.Tick(3*time.Second)
  for _ = range c {
    ctrl.update()
  }
}

func (ctrl *IPDiscoveryController) update() error {
  glog.V(5).Infof("begin update")
  defer glog.V(5).Infof("end update")
  
  arpHandle, err := os.Open("/proc/net/arp")
  if err != nil {
    glog.Warningf(err.Error())
    return err
  }
  defer arpHandle.Close()

  table := []vmapi.ARPEntry{}
  arp := bufio.NewScanner(arpHandle)
  for arp.Scan() {
    l := arp.Text()
    // ignore header
    if strings.HasPrefix(l, "IP") {
      continue
    }
    f := strings.Fields(l)
    // ignore invalid entries
    if len(f) != 6 {
      continue
    }
    // only store entries on the managed bridge
    if f[5] != "br0" {
      continue
    }
    table = append(table, vmapi.ARPEntry{
      IP: f[0],
      HWType: f[1],
      Flags: f[2],
      HWAddress: f[3],
      Mask: f[4],
      Device: f[5],
    })
  }

  curTable, err := ctrl.arpLister.Get(ctrl.nodeName)
  if err == nil {
    arptable := curTable.DeepCopy()
    arptable.Spec.Table = table
    arptable, err = ctrl.crdClient.VirtualmachineV1alpha1().ARPTables().Update(arptable)
    if err != nil {
      glog.Warningf(err.Error())
    }

  } else if !apierrors.IsNotFound(err) {
    glog.V(2).Infof("error getting arptable %s: %v", ctrl.nodeName, err)
    return err

  } else {
    arptable := &vmapi.ARPTable{
      // I shouldn't have to set the type meta, what's wrong with the client?
      TypeMeta: metav1.TypeMeta{
        APIVersion: "vm.rancher.com/v1alpha1",
        Kind: "ARPTable",
      },
      ObjectMeta: metav1.ObjectMeta{
        Name:      ctrl.nodeName,
      },
      Spec: vmapi.ARPTableSpec{
        Table: table,
      },
    }
    arptable, err = ctrl.crdClient.VirtualmachineV1alpha1().ARPTables().Create(arptable)
    if err != nil {
      glog.Warningf(err.Error())
    }
  }

  return err
}

func (ctrl *IPDiscoveryController) enqueueWork(queue workqueue.Interface, obj interface{}) {
  // Beware of "xxx deleted" events
  if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
    obj = unknown.Obj
  }
  objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
  if err != nil {
    glog.Errorf("failed to get key from object: %v", err)
    return
  }
  glog.V(5).Infof("enqueued %q for sync", objName)
  queue.Add(objName)
}

func (ctrl *IPDiscoveryController) arpWorker() {
  workFunc := func() bool {
    keyObj, quit := ctrl.arpQueue.Get()
    if quit {
      return true
    }
    defer ctrl.arpQueue.Done(keyObj)
    key := keyObj.(string)
    glog.V(5).Infof("arpWorker[%s]", key)

    _, name, err := cache.SplitMetaNamespaceKey(key)
    if err != nil {
      glog.V(4).Infof("error getting name of arp table %q to get arp table from informer: %v", key, err)
      return false
    }
    _, err = ctrl.arpLister.Get(name)
    if err == nil {
      // The vm still exists in informer cache, the event must have been
      // add/update/sync
      // ctrl.updateVM(vm)
      return false
    }
    if !apierrors.IsNotFound(err) {
      glog.V(2).Infof("error getting arp table %q from informer: %v", key, err)
      return false
    }

    // The vm is not in informer cache, the event must have been
    // delete
    // ctrl.deleteVM(ns, name)
    return false
  }
  for {
    if quit := workFunc(); quit {
      glog.Infof("arp table worker queue shutting down")
      return
    }
  }
}
