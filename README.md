# kubevirt-force-rw-disk-addon

Allows KubeVirt VMs to start with SCSI write-protected LUNs (e.g., passive DR replicas on Dell Powermax). After a DR role swap, writes succeed immediately without VM restart — matching VMware RDM behavior.

## How it works

QEMU checks `ioctl(BLKROGET)` during block device initialization and refuses to start if the device is read-only. A small shared library intercepts this call and returns `readonly=0`, allowing the VM to start. Writes to the passive LUN reach the array and get rejected — the guest sees I/O errors, which application middleware handles. After role swap, writes succeed immediately.

The library is loaded via `/etc/ld.so.preload` (not the `LD_PRELOAD` environment variable) because `qemu-kvm` has file capabilities that cause glibc to ignore `LD_PRELOAD`.

Add an annotation to your VirtualMachine to enable the bypass:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: my-vm
spec:
  template:
    metadata:
      annotations:
        kubevirt.io/force-rw-disk: "true"
    spec:
      domain:
        devices:
          disks:
            - name: datadisk
              lun:
                bus: scsi
              errorPolicy: report
      volumes:
        - name: datadisk
          persistentVolumeClaim:
            claimName: my-write-protected-lun
```

Setting `errorPolicy: report` is recommended for write-protected LUNs. The default policy (`stop`) pauses the VM on the first write error. With `report`, write errors are passed through to the guest so application middleware can handle them.

The addon provides the following components:

1. **Pod mutating webhook** — intercepts virt-launcher pod creation. When the pod has `kubevirt.io/force-rw-disk: "true"`, it injects an init container and mounts the shared library into the compute container via `/etc/ld.so.preload`
2. **Init container image** — minimal image containing the `blkro_override.so` shared library

## Limitations

- **amd64 only** — the shared library is compiled for x86_64
- The `ld.so.preload` intercept applies to all processes in the compute container, not just QEMU. The intercept only affects `BLKROGET` ioctls, which are harmless for non-QEMU processes
- The webhook has `failurePolicy: Ignore` — if the webhook is unavailable, VMs will start without the bypass

## Build

```bash
make build         # build binaries
make test          # run tests
make images        # build container images
make push          # push container images
```

Override the image registry and tag:

```bash
make images IMAGE_REGISTRY=quay.io/myorg IMAGE_TAG=v1.0.0
make push IMAGE_REGISTRY=quay.io/myorg IMAGE_TAG=v1.0.0
```

## Prerequisites

- KubeVirt 1.0+
- The KubeVirt `Sidecar` feature gate is **not** required

## Install from release

Download the release manifest from the [GitHub Releases](https://github.com/openshift-cnv/kubevirt-force-rw-disk-addon/releases) page and apply it:

**OpenShift:**

```bash
kubectl apply -f https://github.com/openshift-cnv/kubevirt-force-rw-disk-addon/releases/latest/download/force-rw-disk-addon-openshift.yaml
```

**Kubernetes** (requires [cert-manager](https://cert-manager.io/docs/installation/)):

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl apply -f https://github.com/openshift-cnv/kubevirt-force-rw-disk-addon/releases/latest/download/force-rw-disk-addon-kubernetes.yaml
```

To install a specific version, replace `latest` with the version tag (e.g., `v0.1.0`).

## Deploy from source

**OpenShift** (uses serving-cert annotation for TLS, no cert-manager required):

```bash
make deploy-openshift IMAGE_REGISTRY=quay.io/myorg IMAGE_TAG=v1.0.0
```

**Kubernetes** (requires [cert-manager](https://cert-manager.io/docs/installation/)):

```bash
make deploy-kubernetes IMAGE_REGISTRY=quay.io/myorg IMAGE_TAG=v1.0.0
```

## Testing with a simulated write-protected LUN

Scripts in `hack/` create a write-protected SCSI device using the `scsi_debug` kernel module:

```bash
./hack/wp-setup.sh        # create write-protected LUN, PV, PVC
./hack/wp-create-vm.sh    # create VM with the LUN attached
./hack/wp-teardown.sh     # clean up everything
```

Verify the bypass is working:

```bash
kubectl logs $(kubectl get pod -l vm.kubevirt.io/name=wp-test-vm -o name) | grep blkro
```

## Generate release manifests

For CI pipelines that produce release artifacts:

```bash
make manifests-openshift SIDECAR_IMAGE=... WEBHOOK_IMAGE=... > release-openshift.yaml
make manifests-kubernetes SIDECAR_IMAGE=... WEBHOOK_IMAGE=... > release-kubernetes.yaml
```

## Uninstall

```bash
make undeploy-openshift    # OpenShift
make undeploy-kubernetes   # upstream Kubernetes
```
