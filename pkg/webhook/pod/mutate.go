package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	annotationForceRW = "kubevirt.io/force-rw-disk"
	computeContainer  = "compute"
	initContainerName = "force-rw-disk-init"
	volumeName        = "force-rw-disk-lib"
	soMountPath       = "/run/force-rw-disk"
	initMountPath     = "/shared"
	librarySrc        = "/usr/lib64/blkro_override.so"
	libraryName       = "blkro_override.so"
	preloadFileName   = "ld.so.preload"
	defaultInitImage  = "IMAGE_PLACEHOLDER"
	envInitImage      = "SIDECAR_IMAGE"
)

type MutatingHandler struct{}

func NewMutatingHandler() *MutatingHandler {
	return &MutatingHandler{}
}

func (h *MutatingHandler) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("only mutating on create")
	}

	pod := &corev1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to parse pod: %w", err))
	}

	val, ok := pod.Annotations[annotationForceRW]
	if !ok || val != "true" {
		return admission.Allowed("no force-rw-disk annotation")
	}

	computeIdx := -1
	for i, c := range pod.Spec.Containers {
		if c.Name == computeContainer {
			computeIdx = i
			break
		}
	}

	if computeIdx < 0 {
		return admission.Allowed("no compute container found")
	}

	if alreadyInjected(pod) {
		return admission.Allowed("already injected")
	}

	initImage := os.Getenv(envInitImage)
	if initImage == "" {
		initImage = defaultInitImage
	}

	soPath := soMountPath + "/" + libraryName
	preloadPath := initMountPath + "/" + preloadFileName

	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
		Name:  initContainerName,
		Image: initImage,
		Command: []string{"sh", "-c",
			fmt.Sprintf("cp %s %s/%s && echo %s > %s", librarySrc, initMountPath, libraryName, soPath, preloadPath),
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeName, MountPath: initMountPath},
		},
	})

	container := &pod.Spec.Containers[computeIdx]
	container.VolumeMounts = append(container.VolumeMounts,
		corev1.VolumeMount{
			Name:      volumeName,
			MountPath: soMountPath,
		},
		corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/etc/ld.so.preload",
			SubPath:   preloadFileName,
		},
	)

	modifiedRaw, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError,
			fmt.Errorf("failed to marshal modified pod: %w", err))
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, modifiedRaw)
}

func alreadyInjected(pod *corev1.Pod) bool {
	for _, v := range pod.Spec.Volumes {
		if v.Name == volumeName {
			return true
		}
	}
	return false
}
