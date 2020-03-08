
<img src="icons/yago_icon.svg" width="125">

# Yago

__Yago__ is a GitOps operator for Kubernetes distributions. The desired state  of a namespace is kept in a git repository. Yago reads this repository and creates the resources in the namespace it runs in. Objects stay synchronized at all times making sure that changes are only made through commits to the repository. Yago does not need external services or tooling, it is configured using a single custom resource. The sync status, last successful checkout, and any error messages are reflected in this resource as well.

The motivation behind the project is to create a useful, simple, robust operator, and practice coding Go and writing Kubernetes operators in the process.

Yago was built using the [Operator SDK](https://github.com/operator-framework/operator-sdk)  and is currently in a very early stage.  
In it's current state it can:
- Check out a branch to memory
- Create API objects based on the repository
- Reconcile objects modified externally

The following basic features are currently under development:
- Object creation smoke tests and dry runs
- Error reporting
- Cleanup of orphaned objects

Yago is currently tested and developed under OpenShift 4.3.

## Prerequisites  
* docker/podman
* [operator-sdk v0.15.2](https://github.com/operator-framework/operator-sdk/releases/tag/v0.15.2)

## Build
```bash 
cd yago
operator-sdk build <image-repo>:<tag> [--image-builder podman] [--verbose]  
{podman|docker} push <image-repo>:<tag>
```

## Deploy
```bash 
cd yago
oc create -f deploy/service_account.yaml
oc create -f deploy/role.yaml
oc create -f deploy/rolebinding.yaml
oc create -f deploy/crds/yago.aerdei.com_yagos_crd.yaml
sed -i 's/REPLACE_IMAGE/<image-repo>:<tag>/g' deploy/operator.yaml
oc create -f deploy/operator.yaml
```

## Use
Create a Yago CR:
```bash
oc create -f - <<EOF
apiVersion: yago.aerdei.com/v1alpha1
kind: Yago
metadata:
  name: example-yago
spec:
  repository: https://github.com/aerdei/yaml-repo-test
  branchReference: "Master"
EOF
```
