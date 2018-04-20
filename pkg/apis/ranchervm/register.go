package ranchervm

import (
	"fmt"
	"time"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	GroupName = "vm.rancher.com"
)

func createVirtualMachineDefinition() *apiextensionsv1beta1.CustomResourceDefinition {
	var minCpus, maxCpus, minMemoryMB, maxMemoryMB float64
	minCpus = 1.0
	maxCpus = 8.0
	minMemoryMB = 64.0
	maxMemoryMB = 65536.0

	return &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "virtualmachines." + GroupName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   GroupName,
			Version: "v1alpha1",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:     "virtualmachines",
				Singular:   "virtualmachine",
				Kind:       "VirtualMachine",
				ShortNames: []string{"vm", "vms"},
			},
			Scope: apiextensionsv1beta1.ClusterScoped,
			Validation: &apiextensionsv1beta1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
					Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
						"spec": apiextensionsv1beta1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
								"cpus": apiextensionsv1beta1.JSONSchemaProps{
									Type:    "integer",
									Minimum: &minCpus,
									Maximum: &maxCpus,
								},
								"memory_mb": apiextensionsv1beta1.JSONSchemaProps{
									Type:    "integer",
									Minimum: &minMemoryMB,
									Maximum: &maxMemoryMB,
								},
								"image": apiextensionsv1beta1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
	}
}

func CreateVirtualMachineDefinition(clientset apiextensionsclient.Interface) error {
	vm := createVirtualMachineDefinition()
	vm, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(vm)
	switch {
	case err == nil:
		break
	case apierrors.IsAlreadyExists(err):
		return nil
	default:
		return err
	}

	// Wait for CRD to be established
	err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		vm, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().
			Get("virtualmachines."+GroupName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range vm.Status.Conditions {
			switch cond.Type {
			case apiextensionsv1beta1.Established:
				if cond.Status == apiextensionsv1beta1.ConditionTrue {
					break
				}
			case apiextensionsv1beta1.NamesAccepted:
				if cond.Status == apiextensionsv1beta1.ConditionFalse {
					fmt.Printf("Name conflict: %v\n", cond.Reason)
				}
			}
		}
		return false, err
	})
	return err
}

func CreateARPTableDefinition(clientset apiextensionsclient.Interface) error {
	arp := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "arptables." + GroupName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   GroupName,
			Version: "v1alpha1",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:     "arptables",
				Singular:   "arptable",
				Kind:       "ARPTable",
				ShortNames: []string{"arp", "arps"},
			},
			Scope: apiextensionsv1beta1.ClusterScoped,
		},
	}

	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(arp)
	switch {
	case err == nil:
		break
	case apierrors.IsAlreadyExists(err):
		return nil
	default:
		return err
	}

	// Wait for CRD to be established
	err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		arp, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().
			Get("arptables."+GroupName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range arp.Status.Conditions {
			switch cond.Type {
			case apiextensionsv1beta1.Established:
				if cond.Status == apiextensionsv1beta1.ConditionTrue {
					return true, err
				}
			case apiextensionsv1beta1.NamesAccepted:
				if cond.Status == apiextensionsv1beta1.ConditionFalse {
					fmt.Printf("Name conflict: %v\n", cond.Reason)
				}
			}
		}
		return false, err
	})
	return err
}

func CreateCredentialDefinition(clientset apiextensionsclient.Interface) error {
	key := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "credentials." + GroupName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   GroupName,
			Version: "v1alpha1",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:     "credentials",
				Singular:   "credential",
				Kind:       "Credential",
				ShortNames: []string{"creds", "cred"},
			},
			Scope: apiextensionsv1beta1.ClusterScoped,
		},
	}

	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(key)
	switch {
	case err == nil:
		break
	case apierrors.IsAlreadyExists(err):
		return nil
	default:
		return err
	}

	// Wait for CRD to be established
	err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		arp, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().
			Get("arptables."+GroupName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range arp.Status.Conditions {
			switch cond.Type {
			case apiextensionsv1beta1.Established:
				if cond.Status == apiextensionsv1beta1.ConditionTrue {
					return true, err
				}
			case apiextensionsv1beta1.NamesAccepted:
				if cond.Status == apiextensionsv1beta1.ConditionFalse {
					fmt.Printf("Name conflict: %v\n", cond.Reason)
				}
			}
		}
		return false, err
	})
	return err
}
