package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/glog"
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
	Name        string   `json:"name"`
	Cpus        int32    `json:"cpus"`
	Memory      int32    `json:"memory"`
	Image       string   `json:"image"`
	Action      string   `json:"action"`
	PublicKeys  []string `json:"pubkey"`
	HostedNovnc bool     `json:"novnc"`
}

func (s *server) InstanceCreate(w http.ResponseWriter, r *http.Request) {
	var ic InstanceCreate
	switch {
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		r.ParseForm()

		if len(r.PostForm["name"]) != 1 ||
			len(r.PostForm["cpus"]) != 1 ||
			len(r.PostForm["mem"]) != 1 ||
			len(r.PostForm["image"]) != 1 ||
			len(r.PostForm["pubkey"]) < 1 ||
			len(r.PostForm["action"]) != 1 ||
			len(r.PostForm["novnc"]) != 1 {

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		cpus, _ := strconv.Atoi(r.PostForm["cpus"][0])
		mem, _ := strconv.Atoi(r.PostForm["mem"][0])
		ic = InstanceCreate{
			Name:        r.PostForm["name"][0],
			Cpus:        int32(cpus),
			Memory:      int32(mem),
			Image:       r.PostForm["image"][0],
			Action:      r.PostForm["action"][0],
			PublicKeys:  r.PostForm["pubkey"],
			HostedNovnc: (r.PostForm["novnc"][0] == "true"),
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

	if !isValidName(ic.Name) ||
		!isValidCpus(ic.Cpus) ||
		!isValidMemory(ic.Memory) ||
		!isValidImage(ic.Image) ||
		!isValidAction(vmapi.ActionType(ic.Action)) ||
		!isValidPublicKeys(ic.PublicKeys) {

		w.WriteHeader(http.StatusBadRequest)
		return
	}

	vm := &vmapi.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: ic.Name,
		},
		Spec: vmapi.VirtualMachineSpec{
			Cpus:         ic.Cpus,
			MemoryMB:     ic.Memory,
			MachineImage: vmapi.MachineImageType(ic.Image),
			Action:       vmapi.ActionType(ic.Action),
			PublicKeys:   ic.PublicKeys,
			HostedNovnc:  ic.HostedNovnc,
		},
	}

	vm, err := s.vmClient.VirtualmachineV1alpha1().VirtualMachines().Create(vm)
	glog.Infof("%+v", err)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusCreated)
	case apierrors.IsAlreadyExists(err):
		w.WriteHeader(http.StatusConflict)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *server) InstanceDelete(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	if !isValidName(name) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := s.vmClient.VirtualmachineV1alpha1().VirtualMachines().Delete(name, &metav1.DeleteOptions{})
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case apierrors.IsNotFound(err):
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
}

func (s *server) InstanceAction(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	action := mux.Vars(r)["action"]
	actionType := vmapi.ActionType(action)

	if !nameRegexp.MatchString(name) || !isValidAction(actionType) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	vm, err := s.vmLister.Get(name)
	switch {
	case err == nil:
		break
	case apierrors.IsNotFound(err):
		w.WriteHeader(http.StatusNotFound)
		return
	default:
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

	vm2, err = s.vmClient.VirtualmachineV1alpha1().VirtualMachines().Update(vm2)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}
