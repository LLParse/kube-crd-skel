package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachine is a specification for a VirtualMachine resource
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec"`
	Status VirtualMachineStatus `json:"status"`
}

// VirtualMachineSpec is the spec for a VirtualMachine resource
type VirtualMachineSpec struct {
	CpuMillis *int32 `json:"cpuMillis"`
	MemoryMB  *int32 `json:"memoryMB"`
}

// VirtualMachineStatus is the status for a VirtualMachine resource
type VirtualMachineStatus struct {
	Running bool `json:"running"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineList is a list of VirtualMachine resources
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VirtualMachine `json:"items"`
}
