package virtualmachine

import (
	"fmt"
	// "reflect"
	"time"

	api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

const GroupName = "virtualmachines.rancher.com"

func CreateCustomResourceDefinition(clientset apiclient.Interface) error {
	crd := &api.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: GroupName,
		},
		Spec: api.CustomResourceDefinitionSpec{
			Group:   "rancher.com",
			Version: "v1alpha1",
			Names: api.CustomResourceDefinitionNames{
				Plural:     "virtualmachines",
				Singular:   "virtualmachine",
				Kind:       "VirtualMachine",
				ShortNames: []string{"vm"},
			},
			Scope: api.NamespaceScoped,
			// Validation: &api.CustomResourceValidation{
			//   OpenAPIV3Schema: &api.JSONSchemaProps{
			//     Properties: map[string]api.JSONSchemaProps{
			//       "cpu_milli": api.JSONSchemaProps{
			//         Type: "integer",
			//         Minimum: &(float64(500.0)),
			//         Maximum: &8000.0,
			//       },
			//       "memory_mb": api.JSONSchemaProps{
			//         Type: "integer",
			//         Minimum: &512.0,
			//         Maximum: &65536.0,
			//       },
			//     },
			//   },
			// },
		},
	}

	if _, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}

	// Wait for CRD to be established
	if err := wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		crd, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().
			Get(GroupName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		for _, cond := range crd.Status.Conditions {
			switch cond.Type {
			case api.Established:
				if cond.Status == api.ConditionTrue {
					return true, err
				}
			case api.NamesAccepted:
				if cond.Status == api.ConditionFalse {
					fmt.Printf("Name conflict: %v\n", cond.Reason)
				}
			}
		}

		return false, err
	}); err != nil {
		if deleteErr := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().
			Delete(GroupName, nil); deleteErr != nil {
			return errors.NewAggregate([]error{err, deleteErr})
		}
		return err
	}

	return nil
}
