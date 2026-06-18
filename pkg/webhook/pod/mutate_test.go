package pod

import (
	"context"
	"encoding/json"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	jsonpatch "github.com/evanphx/json-patch/v5"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func makePodRequest(annotations map[string]string, containers []corev1.Container) admission.Request {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "virt-launcher-test-xyz",
			Namespace:   "default",
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: containers,
		},
	}
	raw, _ := json.Marshal(pod)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: admissionv1.Create,
			Object: runtime.RawExtension{
				Raw: raw,
			},
			Resource: metav1.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
		},
	}
}

func applyPatches(req admission.Request, resp admission.Response) corev1.Pod {
	patchBytes, err := json.Marshal(resp.Patches)
	Expect(err).NotTo(HaveOccurred())

	patch, err := jsonpatch.DecodePatch(patchBytes)
	Expect(err).NotTo(HaveOccurred())

	modified, err := patch.Apply(req.Object.Raw)
	Expect(err).NotTo(HaveOccurred())

	var result corev1.Pod
	Expect(json.Unmarshal(modified, &result)).To(Succeed())
	return result
}

func findContainer(containers []corev1.Container, name string) *corev1.Container {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}

var _ = Describe("Pod Mutating Webhook", func() {
	var handler *MutatingHandler

	BeforeEach(func() {
		handler = NewMutatingHandler()
		Expect(os.Setenv(envInitImage, "quay.io/test/force-rw-disk-init:latest")).To(Succeed())
		DeferCleanup(os.Unsetenv, envInitImage)
	})

	Context("when pod has force-rw-disk annotation", func() {
		It("should add init container, volume, and mounts", func() {
			req := makePodRequest(
				map[string]string{annotationForceRW: "true"},
				[]corev1.Container{
					{
						Name:    "compute",
						Image:   "registry/virt-launcher:latest",
						Command: []string{"/usr/bin/virt-launcher-monitor"},
						Args:    []string{"--some-flag", "value"},
					},
				},
			)

			resp := handler.Handle(context.Background(), req)
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patches).NotTo(BeEmpty())

			pod := applyPatches(req, resp)

			init := findContainer(pod.Spec.InitContainers, initContainerName)
			Expect(init).NotTo(BeNil())
			Expect(init.Image).To(Equal("quay.io/test/force-rw-disk-init:latest"))

			var hasVolume bool
			for _, v := range pod.Spec.Volumes {
				if v.Name == volumeName {
					Expect(v.EmptyDir).NotTo(BeNil())
					hasVolume = true
				}
			}
			Expect(hasVolume).To(BeTrue())

			compute := findContainer(pod.Spec.Containers, "compute")
			Expect(compute).NotTo(BeNil())

			var hasSoMount, hasPreloadMount bool
			for _, m := range compute.VolumeMounts {
				if m.Name == volumeName && m.MountPath == soMountPath {
					hasSoMount = true
				}
				if m.Name == volumeName && m.MountPath == "/etc/ld.so.preload" && m.SubPath == preloadFileName {
					hasPreloadMount = true
				}
			}
			Expect(hasSoMount).To(BeTrue())
			Expect(hasPreloadMount).To(BeTrue())

			Expect(compute.Command).To(Equal([]string{"/usr/bin/virt-launcher-monitor"}))
			Expect(compute.Args).To(Equal([]string{"--some-flag", "value"}))
		})

		It("should not duplicate when called twice (reinvocation)", func() {
			req := makePodRequest(
				map[string]string{annotationForceRW: "true"},
				[]corev1.Container{
					{
						Name:    "compute",
						Image:   "registry/virt-launcher:latest",
						Command: []string{"/usr/bin/virt-launcher-monitor"},
					},
				},
			)

			resp1 := handler.Handle(context.Background(), req)
			Expect(resp1.Allowed).To(BeTrue())
			Expect(resp1.Patches).NotTo(BeEmpty())

			pod := applyPatches(req, resp1)
			raw2, _ := json.Marshal(pod)
			req2 := req
			req2.Object.Raw = raw2

			resp2 := handler.Handle(context.Background(), req2)
			Expect(resp2.Allowed).To(BeTrue())
			Expect(resp2.Patches).To(BeEmpty())
		})

		It("should allow when no compute container found", func() {
			req := makePodRequest(
				map[string]string{annotationForceRW: "true"},
				[]corev1.Container{
					{Name: "sidecar", Image: "registry/sidecar:latest"},
				},
			)

			resp := handler.Handle(context.Background(), req)
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patches).To(BeEmpty())
		})
	})

	Context("when pod does not have force-rw-disk annotation", func() {
		It("should allow without patching", func() {
			req := makePodRequest(
				map[string]string{},
				[]corev1.Container{
					{Name: "compute", Image: "registry/virt-launcher:latest"},
				},
			)

			resp := handler.Handle(context.Background(), req)
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patches).To(BeEmpty())
		})

		It("should allow when annotation is not true", func() {
			req := makePodRequest(
				map[string]string{annotationForceRW: "false"},
				[]corev1.Container{
					{Name: "compute", Image: "registry/virt-launcher:latest"},
				},
			)

			resp := handler.Handle(context.Background(), req)
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patches).To(BeEmpty())
		})
	})
})
