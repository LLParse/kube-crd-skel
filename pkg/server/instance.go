package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	vmapi "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
)

type InstanceList struct {
	Instances []*vmapi.VirtualMachine `json:"data"`
}

func (s *server) InstanceList(w http.ResponseWriter, r *http.Request) {
	vms, err := s.vmLister.List(labels.Everything())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp, err := json.Marshal(InstanceList{
		Instances: vms,
	})

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

type InstanceCreate struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Cpus      int32  `json:"cpus"`
	Memory    int32  `json:"memory"`
	Image     string `json:"image"`
	Action    string `json:"action"`
}

func (s *server) InstanceCreate(w http.ResponseWriter, r *http.Request) {
	var ic InstanceCreate
	switch {
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		r.ParseForm()
		cpus, _ := strconv.Atoi(r.PostForm["cpus"][0])
		mem, _ := strconv.Atoi(r.PostForm["mem"][0])
		ic = InstanceCreate{
			Namespace: r.PostForm["ns"][0],
			Name:      r.PostForm["name"][0],
			Cpus:      int32(cpus),
			Memory:    int32(mem),
			Image:     r.PostForm["image"][0],
			Action:    r.PostForm["action"][0],
		}
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/json"):
		defer r.Body.Close()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = json.Unmarshal(body, &ic)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO validate result

	// check for conflicts
	vm, err := s.vmLister.VirtualMachines(ic.Namespace).Get(ic.Name)
	if err == nil {
		w.WriteHeader(http.StatusConflict)
		return
	} else if !apierrors.IsNotFound(err) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	vm = &vmapi.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: ic.Name,
		},
		Spec: vmapi.VirtualMachineSpec{
			Cpus:         ic.Cpus,
			MemoryMB:     ic.Memory,
			MachineImage: vmapi.MachineImageType(ic.Image),
			Action:       vmapi.ActionType(ic.Action),
		},
	}

	vm, err = s.vmClient.VirtualmachineV1alpha1().VirtualMachines(ic.Namespace).Create(vm)
	if err == nil {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *server) InstanceDelete(w http.ResponseWriter, r *http.Request) {
	ns := mux.Vars(r)["ns"]
	name := mux.Vars(r)["name"]

	err := s.vmClient.VirtualmachineV1alpha1().VirtualMachines(ns).Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) InstanceAction(w http.ResponseWriter, r *http.Request) {
	ns := mux.Vars(r)["ns"]
	name := mux.Vars(r)["name"]
	action := mux.Vars(r)["action"]

	vm, err := s.vmLister.VirtualMachines(ns).Get(name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	vm2 := vm.DeepCopy()
	vm2.Spec.Action = vmapi.ActionType(action)
	if vm.Spec.Action == vm2.Spec.Action {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	vm2, err = s.vmClient.VirtualmachineV1alpha1().VirtualMachines(ns).Update(vm2)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}
