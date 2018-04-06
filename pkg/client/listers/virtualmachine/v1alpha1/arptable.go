/*
Copyright 2018 Rancher Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file was automatically generated by lister-gen

package v1alpha1

import (
	v1alpha1 "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// ARPTableLister helps list ARPTables.
type ARPTableLister interface {
	// List lists all ARPTables in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.ARPTable, err error)
	// Get retrieves the ARPTable from the index for a given name.
	Get(name string) (*v1alpha1.ARPTable, error)
	ARPTableListerExpansion
}

// aRPTableLister implements the ARPTableLister interface.
type aRPTableLister struct {
	indexer cache.Indexer
}

// NewARPTableLister returns a new ARPTableLister.
func NewARPTableLister(indexer cache.Indexer) ARPTableLister {
	return &aRPTableLister{indexer: indexer}
}

// List lists all ARPTables in the indexer.
func (s *aRPTableLister) List(selector labels.Selector) (ret []*v1alpha1.ARPTable, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.ARPTable))
	})
	return ret, err
}

// Get retrieves the ARPTable from the index for a given name.
func (s *aRPTableLister) Get(name string) (*v1alpha1.ARPTable, error) {
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("arptable"), name)
	}
	return obj.(*v1alpha1.ARPTable), nil
}
