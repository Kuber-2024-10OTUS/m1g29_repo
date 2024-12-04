package controller

import (
    "context"
    ctrl "sigs.k8s.io/controller-runtime"
    corev1 "k8s.io/api/core/v1"
    appsv1 "k8s.io/api/apps/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/util/intstr"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/api/resource"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "sigs.k8s.io/controller-runtime/pkg/log"
    "k8s.io/apimachinery/pkg/runtime"
    "github.com/test/mysql-operator-new/api/v1"
)

type MySQLReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *MySQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1.MySQL{}). 
        Complete(r)
}

func (r *MySQLReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    ctx = context.Background()
    mysql := &v1.MySQL{}
    logger := log.FromContext(ctx)

    if err := r.Get(ctx, req.NamespacedName, mysql); err != nil {
        if errors.IsNotFound(err) {
            logger.Info("MySQL resource not found, deleting associated resources", "name", req.NamespacedName)
            r.deleteResources(req.NamespacedName)
            return reconcile.Result{}, nil
        }
        logger.Error(err, "Failed to get MySQL resource")
        return reconcile.Result{}, err
    }

    exists, err := r.checkDeploymentExists(req.NamespacedName)
    if err != nil {
        return reconcile.Result{}, err
    }
    
    if !exists {
        deployment := r.buildDeployment(mysql)
        if err := controllerutil.SetControllerReference(mysql, deployment, r.Scheme); err != nil {
            return reconcile.Result{}, err
        }
        if err := r.Create(ctx, deployment); err != nil && !errors.IsAlreadyExists(err) {
            return reconcile.Result{}, err
        }
    } else {
        logger.Info("Deployment already exists, skipping creation.", "name", req.NamespacedName)
    }

    service := r.buildService(mysql)
    if err := controllerutil.SetControllerReference(mysql, service, r.Scheme); err != nil {
        return reconcile.Result{}, err
    }
    if err := r.Create(ctx, service); err != nil && !errors.IsAlreadyExists(err) {
        return reconcile.Result{}, err
    }

    pvc := r.buildPVC(mysql)
    if err := controllerutil.SetControllerReference(mysql, pvc, r.Scheme); err != nil {
        return reconcile.Result{}, err
    }
    if err := r.Create(ctx, pvc); err != nil && !errors.IsAlreadyExists(err) {
        return reconcile.Result{}, err
    }

    return reconcile.Result{}, nil
}

func (r *MySQLReconciler) checkDeploymentExists(name types.NamespacedName) (bool, error) {
    deployment := &appsv1.Deployment{}
    err := r.Get(context.Background(), name, deployment)
    if err != nil {
        if errors.IsNotFound(err) {
            return false, nil
        }
        return false, err
    }
    return true, nil
}

func (r *MySQLReconciler) buildDeployment(mysql *v1.MySQL) *appsv1.Deployment {
    labels := map[string]string{"app": mysql.Name}
    return &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mysql.Name,
            Namespace: mysql.Namespace,
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: int32Ptr(1),
            Selector: &metav1.LabelSelector{
                MatchLabels: labels,
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: labels,
                },
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{
                        {
                            Name:  "mysql",
                            Image: "mysql:5.7", // Укажите нужный образ MySQL
                            Ports: []corev1.ContainerPort{
                                {
                                    ContainerPort: 3306,
                                },
                            },
                            Env: []corev1.EnvVar{
                                {
                                    Name:  "MYSQL_PASSWORD",
                                    Value: mysql.Spec.Password,
                                },
                            },
                        },
                    },
                },
            },
        },
    }
}

func (r *MySQLReconciler) buildService(mysql *v1.MySQL) *corev1.Service {
    labels := map[string]string{"app": mysql.Name}
    return &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mysql.Name,
            Namespace: mysql.Namespace,
        },
        Spec: corev1.ServiceSpec{
            Selector: labels,
            Ports: []corev1.ServicePort{
                {
                    Port: 3306,
                    TargetPort: intstr.FromInt(3306),
                },
            },
            Type: corev1.ServiceTypeClusterIP,
        },
    }
}

func (r *MySQLReconciler) buildPVC(mysql *v1.MySQL) *corev1.PersistentVolumeClaim {
    return &corev1.PersistentVolumeClaim{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mysql.Name,
            Namespace: mysql.Namespace,
        },
        Spec: corev1.PersistentVolumeClaimSpec{
            AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
            Resources: corev1.VolumeResourceRequirements{
                Requests: corev1.ResourceList{
                    corev1.ResourceStorage: resource.MustParse(mysql.Spec.StorageSize),
                },
            },
        },
    }
}

func (r *MySQLReconciler) deleteResources(name types.NamespacedName) {
    ctx := context.Background()

    deployment := &appsv1.Deployment{}
    if err := r.Get(ctx, name, deployment); err == nil {
        r.Delete(ctx, deployment)
    }

    service := &corev1.Service{}
    if err := r.Get(ctx, name, service); err == nil {
        r.Delete(ctx, service)
    }

    pvc := &corev1.PersistentVolumeClaim{}
    if err := r.Get(ctx, name, pvc); err == nil {
        r.Delete(ctx, pvc)
    }
}

func int32Ptr(i int32) *int32 {
    return &i
}

