package yago

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	yagov1alpha1 "github.com/aerdei/yago/pkg/apis/yago/v1alpha1"
	"github.com/aerdei/yago/pkg/controller/gitutils"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_yago")
var c controller.Controller

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Yago Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileYago{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	var err error
	c, err = controller.New("yago-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Yago
	err = c.Watch(
		&source.Kind{Type: &yagov1alpha1.Yago{}},
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Ignore updates to CR that are not changing spec
				log.Info("Checking update event")
				unOld, err := runtime.DefaultUnstructuredConverter.ToUnstructured(e.ObjectOld)
				if err != nil {
					return false
				}
				unNew, err := runtime.DefaultUnstructuredConverter.ToUnstructured(e.ObjectNew)
				if err != nil {
					return false
				}
				return !cmp.Equal(unOld["spec"], unNew["spec"])
			},
		})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileYago implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileYago{}

// ReconcileYago reconciles a Yago object
type ReconcileYago struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// variable to track last succesful reference
var (
	ref           *plumbing.Reference
	files         *object.Tree
	deserializer  = serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer()
	currentBranch string
	retryInterval = time.Second * 5
	timeout       = time.Second * 60
)

// Reconcile reads that state of the cluster for a Yago object and makes changes based on the state read
// and what is in the Yago.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileYago) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Yago")
	// Fetch the Yago instance
	instance := &yagov1alpha1.Yago{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if files == nil || instance.Spec.BranchReference != currentBranch {
		reqLogger.Info("Cloning repo")
		if instance.Spec.BranchReference == "" {
			instance.Spec.BranchReference = "Master"
		}
		ref, files, err = gitutils.HandleRepo(instance.Spec.Repository, instance.Spec.BranchReference)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	filesIter := files.Files()
	for {
		f, err := filesIter.Next()
		if err == io.EOF {
			reqLogger.Info("End of list")
			instance.Status.CurrentCommit = ref.String()
			currentBranch = instance.Spec.BranchReference
			if err := r.client.Status().Update(context.TODO(), instance); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		} else if err != nil {
			return reconcile.Result{}, err
		}

		cont, err := f.Contents()
		if err != nil {
			return reconcile.Result{}, err
		}

		unst := &unstructured.Unstructured{}

		_, gvk, err := deserializer.Decode([]byte(cont), nil, unst)
		if err != nil {
			return reconcile.Result{}, err
		}

		name, isNameFound, err := unstructured.NestedString(unst.UnstructuredContent(), "metadata", "name")
		if !isNameFound {
			return reconcile.Result{}, err
		}

		found := &unstructured.Unstructured{}
		found.SetGroupVersionKind(schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind})

		err = r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: request.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			unstructured.SetNestedField(unst.Object, request.Namespace, "metadata", "namespace")
			if err := controllerutil.SetControllerReference(instance, unst, r.scheme); err != nil {
				return reconcile.Result{}, err
			}
			if err := r.client.Create(context.TODO(), unst); err != nil {
				return reconcile.Result{}, err
			}
			c.Watch(&source.Kind{Type: found}, &handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &yagov1alpha1.Yago{},
			})
		} else if err != nil {
			return reconcile.Result{}, err
		} else if !cmp.Equal(found.Object["spec"], unst.Object["spec"]) {
			if result, err := r.mergeObjects(&request, unst, found, instance.Spec.ForceUpdate, reqLogger); err != nil {
				return result, err
			}
		}
	}
}

func (r *ReconcileYago) mergeObjects(
	request *reconcile.Request,
	unst *unstructured.Unstructured,
	found *unstructured.Unstructured,
	forceUpdate bool,
	reqLogger logr.Logger) (reconcile.Result, error) {

	reqLogger.Info("Merging")

	unstructured.SetNestedField(unst.Object, request.Namespace, "metadata", "namespace")

	name, isNameFound, err := unstructured.NestedString(unst.UnstructuredContent(), "metadata", "name")
	if !isNameFound {
		return reconcile.Result{}, err
	}

	constPatch, err := createSpecPatch(found, unst)
	if err != nil {
		return reconcile.Result{}, err
	}
	//Try to patch object
	if err := r.client.Patch(context.TODO(), found.DeepCopy(), constPatch); err != nil {
		//If object cannot be patched, and it's because it has immutable fields, recreate object
		if err.(*errors.StatusError).ErrStatus.Code == 422 &&
			strings.Contains(err.(*errors.StatusError).ErrStatus.Message, "immutable") &&
			forceUpdate {
			reqLogger.Info("ForceUpdate is true")
			//Delete object
			if err := r.client.Delete(context.TODO(), found); err != nil {
				return reconcile.Result{}, err
			}
			//Wait for object to be deleted
			if err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
				getErr := r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: request.Namespace}, found)
				if getErr != nil {
					if errors.IsNotFound(getErr) {
						return true, nil
					}
					return false, getErr
				}
				return false, nil
			}); err != nil {
				return reconcile.Result{}, err
			}
			//Create object again
			reqLogger.Info("Merge-recreate")
			if err := r.client.Create(context.TODO(), unst); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
		reqLogger.Info("ForceUpdate is false")
	}
	return reconcile.Result{}, err
}

func createSpecPatch(found *unstructured.Unstructured, unst *unstructured.Unstructured) (client.Patch, error) {
	patch := found
	patch.Object["spec"] = unst.Object["spec"]
	marshalledPatch, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	return client.ConstantPatch(types.MergePatchType, marshalledPatch), nil
}
