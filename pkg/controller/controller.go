package controller

import (
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	virtualmachinev1alpha1 "github.com/llparse/kube-crd-skel/pkg/client/clientset/versioned/typed/virtualmachine/v1alpha1"
	vmlisterv1alpha1 "github.com/llparse/kube-crd-skel/pkg/client/listers/virtualmachine/v1alpha1"
)

type Controller struct {
	vmClient   virtualmachinev1alpha1.VirtualmachineV1alpha1Interface
	vmInformer cache.SharedInformer
	vmLister   vmlisterv1alpha1.VirtualMachineLister
	vmQueue    workqueue.RateLimitingInterface
}

func New(
	vmClient virtualmachinev1alpha1.VirtualmachineV1alpha1Interface,
	vmInformer cache.SharedInformer,
	vmLister vmlisterv1alpha1.VirtualMachineLister,
	vmQueue workqueue.RateLimitingInterface,
) *Controller {
	return &Controller{
		vmClient:   vmClient,
		vmInformer: vmInformer,
		vmLister:   vmLister,
		vmQueue:    vmQueue,
	}
}

func (c *Controller) Run(stop <-chan struct{}) error {
	c.vmInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { c.addVmToQueue(obj) },
			UpdateFunc: func(_, newObj interface{}) { c.addVmToQueue(newObj) },
		},
	)
	go c.vmInformer.Run(stop)

	// Wait for cache to sync before processing
	if !cache.WaitForCacheSync(stop, c.vmInformer.HasSynced) {
		return errors.New("Failed to sync VirtualMachines")
	}

	go wait.Until(c.processVmQueue, time.Second, stop)

	<-stop
	return nil
}

func (c *Controller) addVmToQueue(obj interface{}) {
	if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
		c.vmQueue.Add(key)
	} else {
		fmt.Println(err.Error())
	}
}

func (c *Controller) processVmQueue() {
	for {
		key, shutdown := c.vmQueue.Get()
		if shutdown {
			return
		}

		err := c.processVm(key.(string))
		if err == nil {
			// c.logger.Debug("successfully processed " + key.(string))
			c.vmQueue.Forget(key)
		} else if c.vmQueue.NumRequeues(key) < 5 {
			c.vmQueue.AddRateLimited(key)
		}
	}
}

func (c *Controller) processVm(vmKey string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(vmKey)
	if err != nil {
		return err
	}

	vm, err := c.vmLister.VirtualMachines(ns).Get(name)
	if apierrors.IsNotFound(err) {
		return nil
	}

	fmt.Printf("Processing vm: %+v\n", vm)
	return nil
}
