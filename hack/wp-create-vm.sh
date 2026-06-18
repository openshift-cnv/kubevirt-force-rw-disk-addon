#!/bin/bash
set -euo pipefail

PVC_NAMESPACE="${PVC_NAMESPACE:-default}"
PVC_NAME="${PVC_NAME:-scsi-wp-pvc}"
VM_NAME="${VM_NAME:-wp-test-vm}"
CONTAINER_DISK="${CONTAINER_DISK:-quay.io/kubevirt/fedora-with-test-tooling:v20240717-a087e7e}"

NODE=$(kubectl get pv scsi-wp-pv -o jsonpath='{.spec.nodeAffinity.required.nodeSelectorTerms[0].matchExpressions[0].values[0]}')

echo "Creating VM $VM_NAME in namespace $PVC_NAMESPACE"
echo "Node:           $NODE"
echo "PVC:            $PVC_NAME"
echo "Container disk: $CONTAINER_DISK"

kubectl apply -f - <<EOF
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: $VM_NAME
  namespace: $PVC_NAMESPACE
spec:
  running: true
  template:
    metadata:
      annotations:
        kubevirt.io/force-rw-disk: "true"
    spec:
      nodeSelector:
        kubernetes.io/hostname: "$NODE"
      domain:
        resources:
          requests:
            memory: 512Mi
        devices:
          rng: {}
          disks:
            - name: disk0
              disk:
                bus: virtio
            - name: lun0
              lun:
                bus: scsi
      volumes:
        - name: disk0
          containerDisk:
            image: $CONTAINER_DISK
        - name: lun0
          persistentVolumeClaim:
            claimName: $PVC_NAME
EOF

echo ""
echo "VM $VM_NAME created. Waiting for it to start..."
echo "Watch with: kubectl get vmi -n $PVC_NAMESPACE $VM_NAME -w"
echo "Console:    virtctl console -n $PVC_NAMESPACE $VM_NAME"
echo "Verify:     kubectl logs \$(kubectl get pod -l vm.kubevirt.io/name=$VM_NAME -o name) | grep blkro"
echo "Delete:     kubectl delete vm -n $PVC_NAMESPACE $VM_NAME"
