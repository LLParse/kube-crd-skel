package vm

import (
	"time"

	"github.com/golang/glog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	vmclientset "github.com/llparse/kube-crd-skel/pkg/client/clientset/versioned"
	virtualmachineinformers "github.com/llparse/kube-crd-skel/pkg/client/informers/externalversions/virtualmachine/v1alpha1"
	vmlisterv1alpha1 "github.com/llparse/kube-crd-skel/pkg/client/listers/virtualmachine/v1alpha1"
)

type VirtualMachineController struct {
	vmClient       vmclientset.Interface
	vmLister       vmlisterv1alpha1.VirtualMachineLister
	vmListerSynced cache.InformerSynced
	vmQueue        workqueue.RateLimitingInterface
}

func NewVirtualMachineController(
	vmClient vmclientset.Interface,
	vmInformer virtualmachineinformers.VirtualMachineInformer,
) *VirtualMachineController {

	ctrl := &VirtualMachineController{
		vmClient: vmClient,
		vmQueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "virtualmachine"),
	}

	vmInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { ctrl.enqueueWork(ctrl.vmQueue, obj) },
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueWork(ctrl.vmQueue, newObj) },
			DeleteFunc: func(obj interface{}) { ctrl.enqueueWork(ctrl.vmQueue, obj) },
		},
	)

	ctrl.vmLister = vmInformer.Lister()
	ctrl.vmListerSynced = vmInformer.Informer().HasSynced

	return ctrl
}

func (ctrl *VirtualMachineController) Run(workers int, stopCh <-chan struct{}) {
	defer ctrl.vmQueue.ShutDown()

	glog.Infof("Starting vm controller")
	defer glog.Infof("Shutting down vm Controller")

	if !cache.WaitForCacheSync(stopCh, ctrl.vmListerSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(ctrl.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (ctrl *VirtualMachineController) enqueueWork(queue workqueue.Interface, obj interface{}) {
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

func (ctrl *VirtualMachineController) worker() {
	workFunc := func() bool {
		keyObj, quit := ctrl.vmQueue.Get()
		if quit {
			return true
		}
		defer ctrl.vmQueue.Done(keyObj)
		key := keyObj.(string)
		glog.V(5).Infof("vmWorker[%s]", key)

		ns, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			glog.V(4).Infof("error getting name of vm %q to get vm from informer: %v", key, err)
			return false
		}
		_, err = ctrl.vmLister.VirtualMachines(ns).Get(name)
		if err == nil {
			// The vm still exists in informer cache, the event must have been
			// add/update/sync
			// ctrl.updateVM(vm)
			return false
		}
		if !apierrors.IsNotFound(err) {
			glog.V(2).Infof("error getting vm %q from informer: %v", key, err)
			return false
		}

		// The volume is not in informer cache, the event must have been
		// delete
		return false
	}
	for {
		if quit := workFunc(); quit {
			glog.Infof("vm worker queue shutting down")
			return
		}
	}
}
