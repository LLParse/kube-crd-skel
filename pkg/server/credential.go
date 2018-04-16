package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

type CredentialCreate struct {
	Name      string `json:"name"`
	PublicKey string `json:"pubkey"`
}

func (s *server) CredentialCreate(w http.ResponseWriter, r *http.Request) {
	var cc CredentialCreate
	switch {
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		r.ParseForm()

		if len(r.PostForm["name"]) != 1 ||
			len(r.PostForm["pubkey"]) != 1 {

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		cc = CredentialCreate{
			Name:      r.PostForm["name"][0],
			PublicKey: r.PostForm["pubkey"][0],
		}
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/json"):
		defer r.Body.Close()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		err = json.Unmarshal(body, &cc)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !isValidName(cc.Name) || !isValidPublicKey(cc.PublicKey) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	cred := &vmapi.Credential{
		// I shouldn't have to set the type meta, what's wrong with the client?
		TypeMeta: metav1.TypeMeta{
			APIVersion: "vm.rancher.com/v1alpha1",
			Kind:       "Credential",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: cc.Name,
		},
		Spec: vmapi.CredentialSpec{
			PublicKey: cc.PublicKey,
		},
	}

	_, err := s.vmClient.VirtualmachineV1alpha1().Credentials().Create(cred)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case apierrors.IsAlreadyExists(err):
		w.WriteHeader(http.StatusConflict)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}
}

func (s *server) CredentialDelete(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	if !nameRegexp.MatchString(name) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := s.vmClient.VirtualmachineV1alpha1().Credentials().Delete(name, &metav1.DeleteOptions{})
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
