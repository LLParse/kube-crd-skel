package vm

import (
  "fmt"
  "strconv"

  "github.com/llparse/kube-crd-skel/pkg/apis/ranchervm/v1alpha1"
  corev1 "k8s.io/api/core/v1"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeEnvVar(name, value string, valueFrom *corev1.EnvVarSource) corev1.EnvVar {
  return corev1.EnvVar{
    Name: name,
    Value: value,
    ValueFrom: valueFrom,
  }
}

func makeEnvVarFieldPath(name, fieldPath string) corev1.EnvVar {
  return corev1.EnvVar{
    Name: name,
    ValueFrom: &corev1.EnvVarSource{
      FieldRef: &corev1.ObjectFieldSelector{
        FieldPath: fieldPath,
      },
    },
  }
}

func makeVolEmptyDir(name string) corev1.Volume {
  return corev1.Volume{
    Name: name,
    VolumeSource: corev1.VolumeSource{
      EmptyDir: &corev1.EmptyDirVolumeSource{},
    },
  }
}

func makeVolHostPath(name, path string) corev1.Volume {
  return corev1.Volume{
    Name: name,
    VolumeSource: corev1.VolumeSource{
      HostPath: &corev1.HostPathVolumeSource{
        Path: path,
      },
    },
  }
}

func makeVolFieldPath(name, path, fieldPath string) corev1.Volume {
  return corev1.Volume{
    Name: name,
    VolumeSource: corev1.VolumeSource{
      DownwardAPI: &corev1.DownwardAPIVolumeSource{
        Items: []corev1.DownwardAPIVolumeFile{
          corev1.DownwardAPIVolumeFile{
            Path: path,
            FieldRef: &corev1.ObjectFieldSelector{
              FieldPath: fieldPath,
            },
          },
        },
      },
    },
  }
}

func makeVolumeMount(name, mountPath, subPath string, readOnly bool) corev1.VolumeMount {
  return corev1.VolumeMount{
    Name: name,
    ReadOnly: readOnly,
    MountPath: mountPath,
    SubPath: subPath,
    MountPropagation: nil,
  }
}

var privileged = true

func makeVMPod(vm *v1alpha1.VirtualMachine, iface string) *corev1.Pod {
  cpu := strconv.Itoa(int(vm.Spec.Cpus))
  mem := strconv.Itoa(int(vm.Spec.MemoryMB))
  image := string(vm.Spec.MachineImage)

  vncProbe := &corev1.Probe{
    Handler: corev1.Handler{
      Exec: &corev1.ExecAction{
        Command: []string{
          "/bin/sh",
          "-c",
          "[ -S /vm/${MY_POD_NAMESPACE}_${MY_POD_NAME}.sock ]",
        },
      },
    },
    InitialDelaySeconds: 2,
    TimeoutSeconds: 2,
    PeriodSeconds: 3,
    SuccessThreshold: 1,
    FailureThreshold: 10,
  }

  return &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
      Name:      vm.Name,
      Labels: map[string]string{
        "app": "ranchervm",
        "name": vm.Name,
        "role": "vm",
      },
      Annotations: map[string]string{
        "cpus": cpu,
        "memory_mb": mem,
        "image": image,
      },
    },
    Spec: corev1.PodSpec{
      Volumes: []corev1.Volume{
        makeVolHostPath("vm-fs", "/tmp/rancher/vm-fs"),
        makeVolEmptyDir("vm-image"),
        makeVolHostPath("vm-socket", "/tmp/rancher/vm-socks"),
        makeVolHostPath("dev-kvm", "/dev/kvm"),
      },
      InitContainers: []corev1.Container{
        corev1.Container{
          Name: "debootstrap",
          Image: "llparse/vm-tools:0.0.1",
          VolumeMounts: []corev1.VolumeMount{
            makeVolumeMount("vm-fs", "/vm-tools", "", false),
          },
        },
      },
      Containers: []corev1.Container{
        corev1.Container{
          Name: "vm",
          Image: fmt.Sprintf("llparse/vm-%s", string(vm.Spec.MachineImage)),
          Command: []string{"/usr/bin/startvm"},
          Env: []corev1.EnvVar{
            makeEnvVar("IFACE", iface, nil),
            makeEnvVar("MEMORY_MB", mem, nil),
            makeEnvVar("CPUS", cpu, nil),
            // Use downward API so we can uniquely name VNC socket
            makeEnvVarFieldPath("MY_POD_NAME", "metadata.name"),
            makeEnvVarFieldPath("MY_POD_NAMESPACE", "metadata.namespace"),
          },
          VolumeMounts: []corev1.VolumeMount{
            makeVolumeMount("vm-image", "/image", "", false),
            makeVolumeMount("dev-kvm", "/dev/kvm", "", false),
            makeVolumeMount("vm-socket", "/vm", "", false),
            makeVolumeMount("vm-fs", "/bin", "bin", true),
            // kubernetes mounts /etc/hosts, /etc/hostname, /etc/resolv.conf
            // we must grant write permissions to /etc to allow these mounts
            makeVolumeMount("vm-fs", "/etc", "etc", false),
            makeVolumeMount("vm-fs", "/lib", "lib", true),
            makeVolumeMount("vm-fs", "/lib64", "lib64", true),
            makeVolumeMount("vm-fs", "/sbin", "sbin", true),
            makeVolumeMount("vm-fs", "/usr", "usr", true),
            makeVolumeMount("vm-fs", "/var", "var", true),            
          },
          LivenessProbe: vncProbe,
          // TODO readinessProbe could be used for checking SSH/RDP/etc
          ReadinessProbe: vncProbe,
          // ImagePullPolicy: corev1.PullPolicy{},
          SecurityContext: &corev1.SecurityContext{
            Privileged: &privileged,
          },
        },
      },
      HostNetwork: true,
    },
  }
}

func makeNovncService(vm *v1alpha1.VirtualMachine) *corev1.Service {
  return &corev1.Service{
    ObjectMeta: metav1.ObjectMeta{
      Name:      vm.Name+"-novnc",
    },
    Spec: corev1.ServiceSpec{
      Ports: []corev1.ServicePort{
        corev1.ServicePort{
          Name: "novnc",
          Port: 6080,
        },
      },
      Selector: map[string]string{
        "app": "ranchervm",
        "name": vm.Name,
        "role": "novnc",
      },
      Type: corev1.ServiceTypeNodePort,
    },
  }
}

func makeNovncPod(vm *v1alpha1.VirtualMachine) *corev1.Pod {
  return &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
      Name:      vm.Name+"-novnc",
      Labels: map[string]string{
        "app": "ranchervm",
        "name": vm.Name,
        "role": "novnc",
      },
    },
    Spec: corev1.PodSpec{
      Volumes: []corev1.Volume{
        makeVolHostPath("vm-socket", "/tmp/rancher/vm-socks"),
        makeVolFieldPath("podinfo", "labels", "metadata.labels"),
      },
      Containers: []corev1.Container{
        corev1.Container{
          Name: "novnc",
          Image: "llparse/novnc:0.0.1",
          Command: []string{"novnc"},
          Env: []corev1.EnvVar{
            makeEnvVarFieldPath("MY_POD_NAMESPACE", "metadata.namespace"),
          },
          VolumeMounts: []corev1.VolumeMount{
            makeVolumeMount("vm-socket", "/vm", "", false),
            makeVolumeMount("podinfo", "/podinfo", "", false),
          },
        },
      },
    },
  }
}
