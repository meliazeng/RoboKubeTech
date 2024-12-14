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

package v1

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var grouplog = logf.Log.WithName("group-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Group) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Group) ValidateCreate() (admission.Warnings, error) {
	grouplog.Info("validate create", "name", r.Name)

	if !isValidGroupType(r.Spec.Type) {
		return nil, fmt.Errorf("invalid group type: %s. Allowed values are: location, department, team", r.Spec.Type)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Group) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	grouplog.Info("validate update", "name", r.Name)
	// Type immutability check
	oldGroup, ok := old.(*Group)
	if !ok {
		return nil, fmt.Errorf("failed to type assert old object to Group")
	}

	if r.Spec.Type != oldGroup.Spec.Type {
		return nil, fmt.Errorf("group type is immutable and cannot be changed after creation")
	}

	return nil, nil
}

// Helper function to check for valid group types
func isValidGroupType(groupType string) bool {
	switch groupType {
	case GroupTypeLocation, GroupTypeSecurity, GroupTypeManagement:
		return true
	}
	return false
}

var _ webhook.Defaulter = &Group{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Group) Default() {
	grouplog.Info("default", "name", r.Name)

	// Set the default value for Type if it's empty
	if r.Spec.Type == "" {
		r.Spec.Type = GroupTypeSecurity // Set your desired default type here
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.

var _ webhook.Validator = &Group{}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Group) ValidateDelete() (admission.Warnings, error) {
	grouplog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
