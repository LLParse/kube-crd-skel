package ranchervm

import (
	"fmt"
	"time"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	GroupName = "vm.rancher.com"
)

func CreateCustomResourceDefinition(clientset apiextensionsclient.Interface) error {
	var minCpus, maxCpus, minMemoryMB, maxMemoryMB float64
	minCpus = 1.0
	maxCpus = 8.0
	minMemoryMB = 64.0
	maxMemoryMB = 65536.0

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
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
				ShortNames: []string{"vm"},
			},
			Scope: apiextensionsv1beta1.NamespaceScoped,
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

	if _, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd); err != nil {
		return err
	}

	// Wait for CRD to be established
	if err := wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		crd, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().
			Get("virtualmachines."+GroupName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		for _, cond := range crd.Status.Conditions {
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
	}); err != nil {
		if deleteErr := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().
			Delete("virtualmachines."+GroupName, nil); deleteErr != nil {
			return errors.NewAggregate([]error{err, deleteErr})
		}
		return err
	}

	return nil
}
