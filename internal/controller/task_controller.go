/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
Todo:
1. Create/update cronjob/job.
2. Call entity pod for tasking
3. update status.
*/

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	v3 "github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	//"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	//"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	robokubev1 "meliazeng/RoboKube/api/v1"
	"meliazeng/RoboKube/internal/controller/handler"
)

// TaskReconciler reconciles a Task object
type TaskReconciler struct {
	log logr.Logger
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const (
	deletionDelay = time.Hour
)

// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=tasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=tasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Task object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *TaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		res = robokubev1.Task{}
		//log = log.FromContext(ctx).WithValues("taskController", "").WithCallDepth(1)
	)

	if err := r.Get(ctx, req.NamespacedName, &res); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("resource could not be found, %w", err))
	}

	var (
		_, cancel = context.WithTimeout(ctx, time.Minute*1)
	)
	defer cancel()

	if result, err := handleFinalizer(ctx, r.Client, &res, finalizer); err != nil || result.Requeue {
		return result, err
	}

	if res.Status.Phase == robokubev1.StatusArchiving {
		r.Client.Delete(ctx, &res)
	}

	if res.Spec.Targets == nil {
		// update status to Running
		if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusCompleted)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}
		return ctrl.Result{}, nil
	}

	var isCompleted = false
	if (res.Spec.RunTime == nil && res.Spec.Schedule == "") || res.Status.Phase == robokubev1.StatusScheduled {
		entityList := &robokubev1.EntityList{}
		r.log.Info("reconciling task", "task", res.Name, "namespace", res.Namespace, "targets", res.Spec.Targets)
		// loop through LastTargets
		for _, lastTarget := range *res.Spec.Targets {
			//var err error
			eList, err := GetEntityListForTarget(ctx, r.Client, res.Namespace, lastTarget)
			if err != nil {
				return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to get entity list: %w", err))
			}
			entityList.Items = append(entityList.Items, eList.Items...)
		}

		// todo: check if all entities involved are in provisoned phase, if not, requeue for next reconcile
		if !res.Spec.UnConditonalRun && !checkEntityPhase(entityList, robokubev1.StatusProvisioned) {
			return reconcile.Result{RequeueAfter: time.Second * 30}, nil
		}

		// update status to Running
		if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusRunning)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}

		if err := r.processEntities(ctx, &res, entityList); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, err)
		}
		// for one time task, continue the reconcile
		if res.Spec.RunTime != nil || res.Spec.Schedule != "" {
			return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
		}
		isCompleted = true
	}

	// start provisoning
	now := time.Now()
	var nextRun, terminationTime time.Time
	if res.Spec.Schedule != "" {
		schedule, err := v3.ParseStandard(res.Spec.Schedule)
		if err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("invalid task schedule: %w", err))
		}
		nextRun = schedule.Next(now)
		// compare nextRun with Spec.RunTime if it is not nil
		if res.Spec.RunTime != nil {
			if nextRun.After(res.Spec.RunTime.Time) {
				isCompleted = true
			}
		}
	} else if res.Spec.RunTime != nil {
		nextRun = res.Spec.RunTime.Time
		if nextRun.Before(now) {
			isCompleted = true
		}
	}

	// if nextRun is nil, it means the task is not scheduled to run, mark it as completed
	if isCompleted {
		if res.Spec.Persistant {
			return ctrl.Result{}, nil
		}
		if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusArchiving)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}
		return ctrl.Result{RequeueAfter: deletionDelay}, nil
	}

	if res.Spec.Offset > 0 {
		terminationTime = nextRun.Add(time.Duration(res.Spec.Offset) * time.Second)
	} else {
		terminationTime = nextRun
		nextRun = nextRun.Add(time.Duration(res.Spec.Offset) * time.Second)
	}

	if err := UpdateStatus(ctx, r.Client, &res, func(obj client.Object) {
		res.Status.ExecuteTime = metav1.NewTime(nextRun)
	}, updateStatusPhase(robokubev1.StatusScheduled)); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
	}

	if nextRun.After(now) {
		// Schedule not yet reached, requeue for later
		return ctrl.Result{RequeueAfter: nextRun.Sub(now)}, nil
	}

	if terminationTime.After(now) {
		if err := UpdateStatus(ctx, r.Client, &res, func(obj client.Object) {
			if len(res.Status.FailedEntities) > 0 {
				res.Status.FailedEntities = nil
			}
		}, updateStatusPhase(robokubev1.StatusScheduled)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to failed state, %w", err))
		}
		// requeue for next reconcile
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.log = ctrl.Log.WithName("controllers").WithName("Task")
	return ctrl.NewControllerManagedBy(mgr).
		For(&robokubev1.Task{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
		}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func checkEntityPhase(entityList *robokubev1.EntityList, phase string) bool {
	for _, entity := range entityList.Items {
		if entity.Status.Phase != phase {
			return false
		}
	}
	return true
}

func (r *TaskReconciler) processEntities(ctx context.Context, res *robokubev1.Task, entityList *robokubev1.EntityList) error {
	log := log.FromContext(ctx).WithValues("taskController", "").WithCallDepth(1)
	failedEntities := make(chan string, len(entityList.Items))
	var wg sync.WaitGroup

	for _, entity := range entityList.Items {
		ReqeustClientProcessor := handler.NewRequestHandler(&res.Spec.Payload, entity.Status.Urls["task"], "post", handler.RequestParams{
			ResourceType:  handler.ResourceTypeEntity,
			TargetName:    entity.Name,
			ErrorCondtion: true,
			DryRun:        res.Spec.DryRun,
		})
		wg.Add(1)

		go func(entity robokubev1.Entity) {
			defer wg.Done()

			for retryCount := 0; retryCount < 3; retryCount++ {
				if _, err := Processor(ctx, r.Client, res, &entity, nil, nil, ReqeustClientProcessor.Handle, setCondition, r.log); err != nil {
					log.Error(err, "Error processing entity (retrying)", "entityName", entity.Name, "retryCount", retryCount)
					time.Sleep(time.Second * 5)

					if retryCount == 2 {
						failedEntities <- entity.Name
						UpdateStatus(ctx, r.Client, &entity, func(obj client.Object) {
							entity.Status.Conditions = append(entity.Status.Conditions, robokubev1.Condition{
								Type:    "Tasking",
								Status:  "False",
								Reason:  "TaskingFailed",
								Message: fmt.Sprintf("Entity %s failed being tasked by Task %s", entity.Name, res.Name),
							})
						})
						r.Recorder.Event(res, "Warning", "TaskingFailed",
							fmt.Sprintf("Entity %s failed being tasked by Task %s", entity.Name, res.Name))
						continue
					}
				}

				r.Recorder.Event(res, "Normal", "Tasking",
					fmt.Sprintf("Entity %s is being tasked by %s - %s ", entity.Name, res.Name, time.Now().Format(time.RFC3339)))

				UpdateStatus(ctx, r.Client, &entity, func(obj client.Object) {
					entity.Status.Conditions = append(entity.Status.Conditions, robokubev1.Condition{
						Type:    "Tasking",
						Status:  "True",
						Reason:  "Tasking",
						Message: fmt.Sprintf("Entity %s is being tasked by Task %s", entity.Name, res.Name),
					})
				})
				break
			}
		}(entity)
	}
	wg.Wait()
	close(failedEntities)

	failedEntitiesList := make([]string, 0)
	for entityName := range failedEntities {
		failedEntitiesList = append(failedEntitiesList, entityName)
	}

	if len(failedEntitiesList) > 0 {
		res.Status.FailedEntities = failedEntitiesList
		if err := UpdateStatus(ctx, r.Client, res, func(obj client.Object) {
			if task, ok := obj.(*robokubev1.Task); ok {
				task.Status.FailedEntities = failedEntitiesList
			}
		}, updateStatusPhase(robokubev1.StatusFailed)); err != nil {
			return fmt.Errorf("failed to set resource to failed state: %w", err)
		}
	} else {
		time.Sleep(time.Second * 10)
		if err := UpdateStatus(ctx, r.Client, res, func(obj client.Object) {
			if task, ok := obj.(*robokubev1.Task); ok {
				task.Status.FailedEntities = nil
				task.Status.CompletedTime = metav1.NewTime(time.Now())
			}
		}, updateStatusPhase(robokubev1.StatusCompleted)); err != nil {
			return fmt.Errorf("failed to set resource to completed state: %w", err)
		}
		log.Info("Successfully reconciled Task", "name", res.Name)
	}

	return nil
}
