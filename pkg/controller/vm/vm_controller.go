package vm

import (
	"fmt"
	"math/rand"
	"reflect"
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

	vmLister         vmlisters.VirtualMachineLister
	vmListerSynced   cache.InformerSynced
	podLister        corelisters.PodLister
	podListerSynced  cache.InformerSynced
	svcLister        corelisters.ServiceLister
	svcListerSynced  cache.InformerSynced
	credLister       vmlisters.CredentialLister
	credListerSynced cache.InformerSynced

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
	credInformer vminformers.CredentialInformer,
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

	ctrl.credLister = credInformer.Lister()
	ctrl.credListerSynced = credInformer.Informer().HasSynced

	return ctrl
}

func (ctrl *VirtualMachineController) Run(workers int, stopCh <-chan struct{}) {
	defer ctrl.vmQueue.ShutDown()

	glog.Infof("Starting vm controller")
	defer glog.Infof("Shutting down vm Controller")

	if !cache.WaitForCacheSync(stopCh, ctrl.vmListerSynced, ctrl.podListerSynced, ctrl.svcListerSynced, ctrl.credListerSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(ctrl.vmWorker, time.Second, stopCh)
	}
	go wait.Until(ctrl.podWorker, time.Second, stopCh)

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
		var publicKeys []*vmapi.Credential
		for _, publicKeyName := range vm.Spec.PublicKeys {
			publicKey, err := ctrl.credLister.Get(publicKeyName)
			if err != nil {
				continue
			}
			publicKeys = append(publicKeys, publicKey)
		}

		_, err = ctrl.kubeClient.CoreV1().Pods(vm.Namespace).Create(makeVMPod(vm, publicKeys, IFACE))
		if err != nil {
			glog.V(2).Infof("Error creating vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
			return
		}
	}

	err = ctrl.updateVMStatus(vm, vm2)
	return
}

func (ctrl *VirtualMachineController) updateNovncPod(vm *vmapi.VirtualMachine) (err error) {
	pod, err := ctrl.podLister.Pods(vm.Namespace).Get(vm.Name + "-novnc")
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
	vm2 := vm.DeepCopy()
	// FIXME shouldn't be hardcoded
	nodeHostname := "kvm.local"

	svc, err := ctrl.svcLister.Services(vm.Namespace).Get(vm.Name + "-novnc")
	switch {
	case err == nil:
		glog.V(2).Infof("Found existing novnc service %s/%s", svc.Namespace, svc.Name)
		vm2.Status.VncEndpoint = fmt.Sprintf("%s:%d", nodeHostname, svc.Spec.Ports[0].NodePort)
	case !apierrors.IsNotFound(err):
		glog.V(2).Infof("error getting novnc service %s/%s: %v", vm.Namespace, vm.Name, err)
		return
	default:
		svc, err = ctrl.kubeClient.CoreV1().Services(vm.Namespace).Create(makeNovncService(vm))
		if err != nil {
			glog.V(2).Infof("Error creating novnc service %s/%s: %v", vm.Namespace, vm.Name, err)
			return
		}
		vm2.Status.VncEndpoint = fmt.Sprintf("%s:%d", nodeHostname, svc.Spec.Ports[0].NodePort)
	}

	err = ctrl.updateVMStatus(vm, vm2)
	return
}

func (ctrl *VirtualMachineController) updateVMStatus(current *vmapi.VirtualMachine, updated *vmapi.VirtualMachine) (err error) {
	if !reflect.DeepEqual(current.Status, updated.Status) {
		updated, err = ctrl.vmClient.VirtualmachineV1alpha1().VirtualMachines(updated.Namespace).Update(updated)
	}
	return
}

func (ctrl *VirtualMachineController) startVM(vm *vmapi.VirtualMachine) (err error) {
	if err = ctrl.updateVmPod(vm); err != nil {
		glog.Warningf("error updating vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
		return
	}
	if err = ctrl.updateNovncPod(vm); err != nil {
		glog.Warningf("error updating novnc pod %s/%s: %v", vm.Namespace, vm.Name, err)
		return
	}
	if err = ctrl.updateNovncService(vm); err != nil {
		glog.Warningf("error updating vm pod %s/%s: %v", vm.Namespace, vm.Name, err)
		return
	}
	return
}

func (ctrl *VirtualMachineController) stopVM(vm *vmapi.VirtualMachine) (err error) {
	vm2 := vm.DeepCopy()
	err = ctrl.deleteVmPod(vm.Namespace, vm.Name)
	switch {
	case err == nil:
		vm2.Status.State = vmapi.StateStopping
	case apierrors.IsNotFound(err):
		vm2.Status.State = vmapi.StateStopped
	default:
		vm2.Status.State = vmapi.StateError
	}
	err = ctrl.updateVMStatus(vm, vm2)
	return
}

func (ctrl *VirtualMachineController) updateVM(vm *vmapi.VirtualMachine) {
	// set the instance id and mac address if not present
	if vm.Status.ID == "" || vm.Status.MAC == "" {
		vm2 := vm.DeepCopy()
		uid := string(vm.UID)
		vm2.Status.ID = fmt.Sprintf("i-%s", uid[:8])
		vm2.Status.MAC = fmt.Sprintf("06:fe:%s:%s:%s:%s", uid[:2], uid[2:4], uid[4:6], uid[6:8])
		ctrl.updateVMStatus(vm, vm2)
		ctrl.updateVM(vm2)
		return
	}

	switch vm.Spec.Action {
	case vmapi.ActionStart:
		ctrl.startVM(vm)
	case vmapi.ActionStop:
		ctrl.stopVM(vm)
	default:
		glog.Warningf("detected vm %s/%s with invalid action \"%s\"", vm.Namespace, vm.Name, vm.Spec.Action)
		return
	}
}

func (ctrl *VirtualMachineController) deleteVmPod(ns, name string) (err error) {
	if _, err = ctrl.podLister.Pods(ns).Get(name); err == nil {
		glog.V(2).Infof("trying to delete vm pod %s/%s", ns, name)
		// TODO soft delete?
		err = ctrl.kubeClient.CoreV1().Pods(ns).Delete(name, &metav1.DeleteOptions{})
	}
	return
}

// do NOT use VM object in method signature
func (ctrl *VirtualMachineController) deleteVM(ns, name string) {
	ctrl.deleteVmPod(ns, name)

	glog.V(2).Infof("trying to delete novnc pod %s/%s", ns, name)
	err := ctrl.kubeClient.CoreV1().Pods(ns).Delete(name+"-novnc", &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		glog.V(2).Infof("error deleting novnc pod %s/%s: %v", ns, name, err)
	}

	glog.V(2).Infof("trying to delete novnc service %s/%s", ns, name)
	err = ctrl.kubeClient.CoreV1().Services(ns).Delete(name+"-novnc", &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		glog.V(2).Infof("error deleting novnc service %s/%s: %v", ns, name, err)
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
		ctrl.deleteVM(ns, name)
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

		ns, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			glog.V(4).Infof("error getting name of vm %q to get vm from informer: %v", key, err)
			return false
		}
		_, err = ctrl.podLister.Pods(ns).Get(name)
		if err == nil {
			glog.V(5).Infof("enqueued %q for sync", keyObj)
			ctrl.vmQueue.Add(keyObj)
		} else if apierrors.IsNotFound(err) {
			glog.V(5).Infof("enqueued %q for sync", keyObj)
			ctrl.vmQueue.Add(keyObj)
		} else {
			glog.Warningf("error getting pod %q from informer: %v", key, err)
		}

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
		if app, ok := pod.Labels["app"]; ok && app == "ranchervm" {
			if role, ok := pod.Labels["role"]; ok && role == "vm" {
				return true
			}
		}
	}
	return false
}
