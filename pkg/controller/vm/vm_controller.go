package vm

import (
	"strconv"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/llparse/kube-crd-skel/pkg/apis/ranchervm"
	vmapi "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
	vmclientset "github.com/llparse/kube-crd-skel/pkg/client/clientset/versioned"
	vminformers "github.com/llparse/kube-crd-skel/pkg/client/informers/externalversions/virtualmachine/v1alpha1"
	vmlisters "github.com/llparse/kube-crd-skel/pkg/client/listers/virtualmachine/v1alpha1"
)

type VirtualMachineController struct {
	vmClient        vmclientset.Interface
	kubeClient      kubernetes.Interface
	vmLister        vmlisters.VirtualMachineLister
	vmListerSynced  cache.InformerSynced
	podLister       corelisters.PodLister
	podListerSynced cache.InformerSynced
	vmQueue         workqueue.RateLimitingInterface
}

func NewVirtualMachineController(
	vmClient vmclientset.Interface,
	kubeClient kubernetes.Interface,
	vmInformer vminformers.VirtualMachineInformer,
	podInformer coreinformers.PodInformer,
) *VirtualMachineController {

	ctrl := &VirtualMachineController{
		vmClient:   vmClient,
		kubeClient: kubeClient,
		vmQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "virtualmachine"),
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

	ctrl.podLister = podInformer.Lister()
	ctrl.podListerSynced = podInformer.Informer().HasSynced

	return ctrl
}

func (ctrl *VirtualMachineController) Run(workers int, stopCh <-chan struct{}) {
	defer ctrl.vmQueue.ShutDown()

	glog.Infof("Starting vm controller")
	defer glog.Infof("Shutting down vm Controller")

	if !cache.WaitForCacheSync(stopCh, ctrl.vmListerSynced, ctrl.podListerSynced) {
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

func (ctrl *VirtualMachineController) updateVM(vm *vmapi.VirtualMachine) {
	// Find pod associated with the VM
	pod, err := ctrl.podLister.Pods(vm.Namespace).Get(vm.Name)
	if err == nil {
		// Update pod (what vm spec updates can we support?)
		glog.V(2).Infof("Pod %s/%s already exists", pod.Namespace, pod.Name)
		return
	}
	if !apierrors.IsNotFound(err) {
		glog.V(2).Infof("error getting pod %s/%s from informer: %v", vm.Namespace, vm.Name, err)
		return
	}

	// Create pod with vm spec
	// FIXME
	pod, err = ctrl.kubeClient.CoreV1().Pods(vm.Namespace).Create(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vm.Name,
			Namespace: vm.Namespace,
			Labels: map[string]string{
				"type": "vm",
			},
			Annotations: map[string]string{
				ranchervm.GroupName + "/cpu_milli": strconv.Itoa(int(vm.Spec.CpuMillis)),
				ranchervm.GroupName + "/memory_mb": strconv.Itoa(int(vm.Spec.MemoryMB)),
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				corev1.Container{
					Name:  "vm-in-a-pod",
					Image: "alpine:3.7",
					Args:  []string{"sleep", "999999"},
				},
			},
		},
	})
	if err != nil {
		glog.V(2).Infof("Error creating pod %s/%s: %v", vm.Namespace, vm.Name, err)
		return
	}
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
		vm, err := ctrl.vmLister.VirtualMachines(ns).Get(name)
		if err == nil {
			// The vm still exists in informer cache, the event must have been
			// add/update/sync
			ctrl.updateVM(vm)
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
