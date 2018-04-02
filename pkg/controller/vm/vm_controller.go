package vm

import (
	"math/rand"
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

	vmapi "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
	vmclientset "github.com/llparse/kube-crd-skel/pkg/client/clientset/versioned"
	vminformers "github.com/llparse/kube-crd-skel/pkg/client/informers/externalversions/virtualmachine/v1alpha1"
	vmlisters "github.com/llparse/kube-crd-skel/pkg/client/listers/virtualmachine/v1alpha1"
)

var IFACE = "ens33"

type VirtualMachineController struct {
	vmClient   vmclientset.Interface
	kubeClient kubernetes.Interface

	vmLister        vmlisters.VirtualMachineLister
	vmListerSynced  cache.InformerSynced
	podLister       corelisters.PodLister
	podListerSynced cache.InformerSynced
	svcLister       corelisters.ServiceLister
	svcListerSynced cache.InformerSynced

	vmQueue  workqueue.RateLimitingInterface
	podQueue workqueue.RateLimitingInterface
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func NewVirtualMachineController(
	vmClient vmclientset.Interface,
	kubeClient kubernetes.Interface,
	vmInformer vminformers.VirtualMachineInformer,
	podInformer coreinformers.PodInformer,
	svcInformer coreinformers.ServiceInformer,
) *VirtualMachineController {

	ctrl := &VirtualMachineController{
		vmClient:   vmClient,
		kubeClient: kubeClient,
		vmQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "virtualmachine"),
		podQueue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pod"),
	}

	vmInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { ctrl.enqueueWork(ctrl.vmQueue, obj) },
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueWork(ctrl.vmQueue, newObj) },
			DeleteFunc: func(obj interface{}) { ctrl.enqueueWork(ctrl.vmQueue, obj) },
		},
	)

	podInformer.Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: ctrl.podFilterFunc,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    func(obj interface{}) { ctrl.enqueueWork(ctrl.podQueue, obj) },
				UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueWork(ctrl.podQueue, newObj) },
				DeleteFunc: func(obj interface{}) { ctrl.enqueueWork(ctrl.podQueue, obj) },
			},
		},
	)

	// TODO handle service resource events

	ctrl.vmLister = vmInformer.Lister()
	ctrl.vmListerSynced = vmInformer.Informer().HasSynced

	ctrl.podLister = podInformer.Lister()
	ctrl.podListerSynced = podInformer.Informer().HasSynced

	ctrl.svcLister = svcInformer.Lister()
	ctrl.svcListerSynced = svcInformer.Informer().HasSynced

	return ctrl
}

func (ctrl *VirtualMachineController) Run(workers int, stopCh <-chan struct{}) {
	defer ctrl.vmQueue.ShutDown()

	glog.Infof("Starting vm controller")
	defer glog.Infof("Shutting down vm Controller")

	if !cache.WaitForCacheSync(stopCh, ctrl.vmListerSynced, ctrl.podListerSynced, ctrl.svcListerSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(ctrl.vmWorker, time.Second, stopCh)
	}
	// go wait.Until(ctrl.podWorker, time.Second, stopCh)

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

func (ctrl *VirtualMachineController) updateVmPod(vm *vmapi.VirtualMachine) (err error) {
	vm2 := vm.DeepCopy()
	vm2.Status.State = vmapi.StatePending

	pod, err := ctrl.podLister.Pods(vm.Namespace).Get(vm.Name)
	if err == nil {
		glog.V(2).Infof("Found existing vm pod %s/%s", pod.Namespace, pod.Name)
		// TODO check the pod against the current spec and update, if necessary
		if pod.DeletionTimestamp != nil {
			vm2.Status.State = vmapi.StateStopping
		}
		for _, c := range pod.Status.Conditions {
			if c.Type != corev1.PodReady {
				continue
			}
			// when pod ready is true, our readinessProbe succeeded
			if c.Status == corev1.ConditionTrue {
				vm2.Status.State = vmapi.StateRunning
			}
		}
	} else if !apierrors.IsNotFound(err) {
		glog.V(2).Infof("error getting vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
		return
	} else {
		_, err = ctrl.kubeClient.CoreV1().Pods(vm.Namespace).Create(makeVMPod(vm, IFACE))
		if err != nil {
			glog.V(2).Infof("Error creating vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
			return
		}
	}

	if vm.Status.State != vm2.Status.State {
		vm2, err = ctrl.vmClient.VirtualmachineV1alpha1().VirtualMachines(vm2.Namespace).Update(vm2)
	}
	return
}

func (ctrl *VirtualMachineController) updateNovncPod(vm *vmapi.VirtualMachine) (err error) {
	pod, err := ctrl.podLister.Pods(vm.Namespace).Get(vm.Name+"-novnc")
	switch {
	case err == nil:
		glog.V(2).Infof("Found existing novnc pod %s/%s", pod.Namespace, pod.Name)
	case !apierrors.IsNotFound(err):
		glog.V(2).Infof("error getting novnc pod %s/%s: %v", vm.Namespace, vm.Name, err)
		return
	default:
		_, err = ctrl.kubeClient.CoreV1().Pods(vm.Namespace).Create(makeNovncPod(vm))
		if err != nil {
			glog.V(2).Infof("Error creating novnc pod %s/%s: %v", vm.Namespace, vm.Name, err)
			return
		}
	}
	return
}

func (ctrl *VirtualMachineController) updateNovncService(vm *vmapi.VirtualMachine) (err error) {
	svc, err := ctrl.svcLister.Services(vm.Namespace).Get(vm.Name+"-novnc")
	switch {
	case err == nil:
		glog.V(2).Infof("Found existing novnc service %s/%s", svc.Namespace, svc.Name)
	case !apierrors.IsNotFound(err):
		glog.V(2).Infof("error getting novnc service %s/%s: %v", vm.Namespace, vm.Name, err)
		return
	default:
		_, err = ctrl.kubeClient.CoreV1().Services(vm.Namespace).Create(makeNovncService(vm))
		if err != nil {
			glog.V(2).Infof("Error creating novnc service %s/%s: %v", vm.Namespace, vm.Name, err)
			return
		}
	}
	return
}

func (ctrl *VirtualMachineController) updateVM(vm *vmapi.VirtualMachine) {
	switch vm.Spec.Action {
	case vmapi.ActionStart:
		if err := ctrl.updateVmPod(vm); err != nil {
			glog.Warningf("error updating vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
			return
		}
		if err := ctrl.updateNovncPod(vm); err != nil {
			glog.Warningf("error updating novnc pod %s/%s: %v", vm.Namespace, vm.Name, err)
			return
		}
		if err := ctrl.updateNovncService(vm); err != nil {
			glog.Warningf("error updating vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
			return
		}

	case vmapi.ActionStop:
		ctrl.deleteVmPod(vm)
	default:
		glog.Warningf("detected vm %s/%s with invalid action \"%s\"", vm.Namespace, vm.Name, vm.Spec.Action)
		return
	}
}

func (ctrl *VirtualMachineController) deleteVmPod(vm *vmapi.VirtualMachine) (err error) {
	vm2 := vm.DeepCopy()

	_, err = ctrl.podLister.Pods(vm.Namespace).Get(vm.Name)
	if err == nil {
		glog.V(2).Infof("deleting vm pod %s/%s", vm.Namespace, vm.Name)

		err = ctrl.kubeClient.CoreV1().Pods(vm.Namespace).Delete(vm.Name, &metav1.DeleteOptions{})
		if err == nil {
			vm2.Status.State = vmapi.StateTerminating
		} else if err != nil && apierrors.IsNotFound(err) {
			vm2.Status.State = vmapi.StateTerminated			
		} else {
			glog.Warningf("error deleting vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
			vm2.Status.State = vmapi.StateError
		}
	} else if apierrors.IsNotFound(err) {
		vm2.Status.State = vmapi.StateTerminated
	} else {
		glog.Warningf("error getting vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
		vm2.Status.State = vmapi.StateError
	}

	if vm.Status.State != vm2.Status.State {
		vm2, err = ctrl.vmClient.VirtualmachineV1alpha1().VirtualMachines(vm2.Namespace).Update(vm2)
	}
	return
}

func (ctrl *VirtualMachineController) deleteVM(vm *vmapi.VirtualMachine) {
	ctrl.deleteVmPod(vm)

	glog.V(2).Infof("deleting novnc pod %s/%s", vm.Namespace, vm.Name)
	err := ctrl.kubeClient.CoreV1().Pods(vm.Namespace).Delete(vm.Name+"-novnc", &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		glog.V(2).Infof("error deleting novnc pod %s/%s: %v", vm.Namespace, vm.Name, err)
	}

	glog.V(2).Infof("deleting novnc service %s/%s", vm.Namespace, vm.Name)
	err = ctrl.kubeClient.CoreV1().Services(vm.Namespace).Delete(vm.Name+"-novnc", &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		glog.V(2).Infof("error deleting novnc service %s/%s: %v", vm.Namespace, vm.Name, err)
	}

	// TODO suppress podInformer from receiving delete event and subsequently
	// requeueing the VM
}

func (ctrl *VirtualMachineController) vmWorker() {
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

		// The vm is not in informer cache, the event must have been
		// delete
		ctrl.deleteVM(vm)
		return false
	}
	for {
		if quit := workFunc(); quit {
			glog.Infof("vm worker queue shutting down")
			return
		}
	}
}

func (ctrl *VirtualMachineController) podWorker() {
	workFunc := func() bool {
		keyObj, quit := ctrl.podQueue.Get()
		if quit {
			return true
		}
		defer ctrl.podQueue.Done(keyObj)
		key := keyObj.(string)
		glog.V(5).Infof("podWorker[%s]", key)


		glog.V(5).Infof("enqueued %q for sync", keyObj)
		ctrl.vmQueue.Add(keyObj)
		return false
	}
	for {
		if quit := workFunc(); quit {
			glog.Infof("pod worker queue shutting down")
			return
		}
	}
}

func (ctrl *VirtualMachineController) podFilterFunc(obj interface{}) bool {
	if pod, ok := obj.(*corev1.Pod); ok {
		if podType, ok := pod.Labels["app"]; ok && podType == "ranchervm" {
			return true
		}
	}
	return false
}
