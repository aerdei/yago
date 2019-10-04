package yago

import (
	"context"
	"io"

	yagov1alpha1 "github.com/aerdei/yago/pkg/apis/yago/v1alpha1"
	"github.com/aerdei/yago/pkg/controller/gitutils"
	appsv1 "github.com/openshift/api/apps/v1"
	buildv1 "github.com/openshift/api/build/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_yago")

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
	c, err := controller.New("yago-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Yago
	err = c.Watch(&source.Kind{Type: &yagov1alpha1.Yago{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Yago
	err = c.Watch(&source.Kind{Type: &appsv1.DeploymentConfig{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &yagov1alpha1.Yago{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &buildv1.BuildConfig{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &yagov1alpha1.Yago{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &yagov1alpha1.Yago{},
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

// Reconcile reads that state of the cluster for a Yago object and makes changes based on the state read
// and what is in the Yago.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
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
	files, err := gitutils.HandleRepo(instance.Spec.Repository)
	if err != nil {
		return reconcile.Result{}, err
	}
	for {
		f, err := files.Next()
		if err == io.EOF {
			return reconcile.Result{}, nil
		} else if err != nil {
			return reconcile.Result{}, err
		}

		cont, err := f.Contents()
		if err != nil {
			return reconcile.Result{}, err
		}
		deserializer := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer()
		obj, _, err := deserializer.Decode([]byte(cont), nil, nil)
		if err != nil {
			return reconcile.Result{}, err
		}

		switch obj.(type) {
		case *appsv1.DeploymentConfig:
			dc := &appsv1.DeploymentConfig{}
			_, _, err := deserializer.Decode([]byte(cont), nil, dc)
			dc.ObjectMeta.Namespace = request.Namespace
			if err != nil {
				return reconcile.Result{}, err
			}
			found := &appsv1.DeploymentConfig{}
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: dc.Name, Namespace: dc.Namespace}, found)
			if err != nil && errors.IsNotFound(err) {
				reqLogger.Info("Creating a new dc", "dc.Namespace", dc.Namespace, "dc.Name", dc.Name)
				if err := controllerutil.SetControllerReference(instance, dc, r.scheme); err != nil {
					return reconcile.Result{}, err
				}
				err = r.client.Create(context.TODO(), dc)
				if err != nil {
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, nil
			} else if err != nil {
				return reconcile.Result{}, err
			}
		case *buildv1.BuildConfig:
			bc := &buildv1.BuildConfig{}
			_, _, err := deserializer.Decode([]byte(cont), nil, bc)
			bc.ObjectMeta.Namespace = request.Namespace
			if err != nil {
				return reconcile.Result{}, err
			}
			found := &buildv1.BuildConfig{}
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: bc.Name, Namespace: bc.Namespace}, found)
			if err != nil && errors.IsNotFound(err) {
				reqLogger.Info("Creating a new bc", "bc.Namespace", bc.Namespace, "bc.Name", bc.Name)
				if err := controllerutil.SetControllerReference(instance, bc, r.scheme); err != nil {
					return reconcile.Result{}, err
				}
				err = r.client.Create(context.TODO(), bc)
				if err != nil {
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, nil
			} else if err != nil {
				return reconcile.Result{}, err
			}
		case *corev1.Service:
			svc := &corev1.Service{}
			_, _, err := deserializer.Decode([]byte(cont), nil, svc)
			svc.ObjectMeta.Namespace = request.Namespace
			if err != nil {
				return reconcile.Result{}, err
			}
			found := &corev1.Service{}
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
			if err != nil && errors.IsNotFound(err) {
				reqLogger.Info("Creating a new svc", "svc.Namespace", svc.Namespace, "svc.Name", svc.Name)
				if err := controllerutil.SetControllerReference(instance, svc, r.scheme); err != nil {
					return reconcile.Result{}, err
				}
				err = r.client.Create(context.TODO(), svc)
				if err != nil {
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, nil
			} else if err != nil {
				return reconcile.Result{}, err
			}
		default:
			return reconcile.Result{}, err
		}

	}
}
