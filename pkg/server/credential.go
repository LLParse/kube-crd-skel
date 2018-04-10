package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	vmapi "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
)

type CredentialList struct {
	Credentials []*vmapi.Credential `json:"data"`
}

func (s *server) CredentialList(w http.ResponseWriter, r *http.Request) {
	creds, err := s.credLister.List(labels.Everything())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp, err := json.Marshal(CredentialList{
		Credentials: creds,
	})

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func (s *server) CredentialDelete(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	err := s.vmClient.VirtualmachineV1alpha1().Credentials().Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}
