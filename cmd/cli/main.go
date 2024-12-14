package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	robokubev1 "meliazeng/RoboKube/api/v1"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "robo",
		Short: "A brief description of your application",
		Long:  `A CLI tool to interact with the myapi service.`,
	}

	// Kubernetes client setup
	var kubeconfig string
	if home := homeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	} else {
		kubeconfig = ""
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(robokubev1.AddToScheme(scheme))

	// Create a new client.Client
	kubeClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		panic(err.Error())
	}

	// Load the raw config
	rawConfig, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// Get the current context name
	currentContext := rawConfig.CurrentContext

	// Group Command
	// kubectl robo model <modelName> --sts <stsName> --replicas <replicas> --fqdn <domain>
	var modelCmd = &cobra.Command{
		Use:   "model <modelName>",
		Short: "Create a new model",
		Args:  cobra.ExactArgs(1), // Requires exactly one argument: groupName
		Run: func(cmd *cobra.Command, args []string) {
			modelName := args[0]
			stsName, _ := cmd.Flags().GetString("sts")
			replicas, _ := cmd.Flags().GetInt32("replicas")
			fqdn, _ := cmd.Flags().GetString("fqdn")
			namespace, _ := cmd.Flags().GetString("namespace")
			if namespace == "" {
				namespace = rawConfig.Contexts[currentContext].Namespace
			}

			fmt.Println("Creating model:", modelName)
			fmt.Println("Existing sts:", stsName)
			fmt.Println("set replicas:", replicas)
			fmt.Println("set fqdn:", fqdn)
			// Implement logic to call kubectl robo create group ...
			// Create group object (replace with your actual object definition)
			model := &robokubev1.Model{ // Replace v1 with your API group version
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace, // Replace with desired namespace
				},
				Spec: robokubev1.ModelSpec{ // Replace with your Spec definition
					Description:     "",
					StatefulSetName: stsName,
					TargetReplicas:  &replicas,
					IngressUrl:      fqdn,
					// ... other fields
				},
			}

			if err := kubeClient.Create(context.TODO(), model); err != nil {
				panic(err.Error())
			}
			fmt.Println("Model created/patch successfully!")
		},
	}
	modelCmd.Flags().StringP("sts", "s", "", "StatefulSet name")
	modelCmd.Flags().StringP("fqdn", "i", "", "access url")
	modelCmd.Flags().Int32P("replicas", "r", 0, "Number of the entities")
	modelCmd.Flags().StringP("namespace", "n", "", "Namespace of the model")
	_ = modelCmd.MarkFlagRequired("sts")
	_ = modelCmd.MarkFlagRequired("fqdn")
	rootCmd.AddCommand(modelCmd)

	// Entity Command: create entity with model and group
	// kubectl robo entity <entityName> --model <modelName> --addGroup <groupName> --removeGroup <groupName>
	var entityCmd = &cobra.Command{
		Use:   "entity <entityName>",
		Short: "Create a new entity",
		Args:  cobra.ExactArgs(1), // Requires exactly one argument: entityName
		Run: func(cmd *cobra.Command, args []string) {
			entityName := args[0]
			modelName, _ := cmd.Flags().GetString("model")
			addGroupName, _ := cmd.Flags().GetString("addGroup")
			removeGroupName, _ := cmd.Flags().GetString("removeGroup")
			namespace, _ := cmd.Flags().GetString("namespace")
			identity, _ := cmd.Flags().GetString("identity")
			privileged, _ := cmd.Flags().GetBool("privileged")
			usage, _ := cmd.Flags().GetString("usage")
			// if namespace is not provided, use the current namespace
			if namespace == "" {
				namespace = rawConfig.Contexts[currentContext].Namespace
			}
			if modelName != "" {
				fmt.Println("Creating entity:", entityName)
				fmt.Println("Model:", modelName)
				// Create entity object (replace with your actual object definition)
				entity := &robokubev1.Entity{
					ObjectMeta: metav1.ObjectMeta{
						Name:      entityName,
						Namespace: namespace,
					},
					Spec: robokubev1.EntitySpec{ // Replace with your Spec definition
						Model:      modelName,
						Usage:      usage,
						Identity:   identity,
						Privileged: privileged,
						// ... other fields
					},
				}

				if err := kubeClient.Create(context.TODO(), entity); err != nil {
					panic(err.Error())
				}

				fmt.Println("Entity created successfully!")
			}

			if addGroupName != "" {
				// add entity to group
				group := &robokubev1.Group{}
				typeName := types.NamespacedName{
					Namespace: namespace,
					Name:      addGroupName,
				}
				// get current group
				if err := kubeClient.Get(context.TODO(), typeName, group); err != nil {
					panic(err.Error())
				}
				// update group with entity, replace lastTargets with entityName
				group.Spec.LastTargets = &[]robokubev1.TargetRef{{Name: entityName, Type: robokubev1.TargetTypeEntity, Action: robokubev1.ResourceLabelAdd}}
				if err := kubeClient.Update(context.TODO(), group); err != nil {
					panic(err.Error())
				}
				fmt.Printf("entity %s add to group %s\n", entityName, addGroupName)
			}

			if removeGroupName != "" {
				// remove entity from group
				group := &robokubev1.Group{}
				typeName := types.NamespacedName{
					Namespace: namespace,
					Name:      removeGroupName,
				}
				// get current group
				if err := kubeClient.Get(context.TODO(), typeName, group); err != nil {
					panic(err.Error())
				}
				// update group with entity, replace lastTargets with entityName
				group.Spec.LastTargets = &[]robokubev1.TargetRef{{Name: entityName, Type: robokubev1.TargetTypeEntity, Action: robokubev1.ResourceLabelDelete}}
				if err := kubeClient.Update(context.TODO(), group); err != nil {
					panic(err.Error())
				}
				fmt.Printf("entity %s removed from group %s\n", entityName, removeGroupName)
			}
		},
	}
	entityCmd.Flags().StringP("model", "m", "", "Name of the model")
	entityCmd.Flags().StringP("identity", "i", "", "Identity of the entity")
	entityCmd.Flags().BoolP("privileged", "p", false, "Privileged of the entity")
	entityCmd.Flags().StringP("usage", "u", "", "Usage of the entity")
	entityCmd.Flags().StringP("namespace", "n", "", "Namespace of the entity")
	entityCmd.Flags().StringP("addGroup", "a", "", "Add to a group")
	entityCmd.Flags().StringP("removeGroup", "r", "", "Remove from a group")
	rootCmd.AddCommand(entityCmd)

	// Group Command
	// kubectl robo group <groupName> --type <groupType> --addGroup <groupName>  --removeGroup <groupName>
	var groupCmd = &cobra.Command{
		Use:   "group <groupName>",
		Short: "Create a new group",
		Args:  cobra.ExactArgs(1), // Requires exactly one argument: groupName
		Run: func(cmd *cobra.Command, args []string) {
			groupName := args[0]
			groupType, _ := cmd.Flags().GetString("type")
			namespace, _ := cmd.Flags().GetString("namespace")
			addGroupName, _ := cmd.Flags().GetString("addGroup")
			removeGroupName, _ := cmd.Flags().GetString("removeGroup")
			if namespace == "" {
				namespace = rawConfig.Contexts[currentContext].Namespace
			}

			if groupType != "" {
				//check if group already exits
				// Check if group exists
				existingGroup := &robokubev1.Group{}
				if err := kubeClient.Get(context.TODO(), types.NamespacedName{
					Namespace: namespace,
					Name:      groupName,
				}, existingGroup); err == nil {
					// Group exists, list entities
					entityList := &robokubev1.EntityList{}
					labelSelector, err := labels.Parse(robokubev1.RoboKubeGroupKey + groupName)
					if err != nil {
						panic(fmt.Errorf("failed to create label selector: %w", err))
					}

					if err := kubeClient.List(context.TODO(), entityList, &client.ListOptions{
						Namespace:     namespace,
						LabelSelector: labelSelector,
					}); err != nil {
						panic(fmt.Errorf("failed to list entities: %w", err))
					}

					fmt.Printf("Found existing group %s with %d entities:\n", groupName, len(entityList.Items))
					for _, entity := range entityList.Items {
						fmt.Printf("- %s\n", entity.Name)
					}
					return
				}

				fmt.Println("Creating group:", groupName)
				fmt.Println("Type:", groupType)
				// Implement logic to call kubectl robo create group ...
				// Create group object (replace with your actual object definition)
				group := &robokubev1.Group{ // Replace v1 with your API group version
					ObjectMeta: metav1.ObjectMeta{
						Name:      groupName,
						Namespace: namespace, // Replace with desired namespace
					},
					Spec: robokubev1.GroupSpec{ // Replace with your Spec definition
						Type: groupType,
						// ... other fields
					},
				}

				// Create the group using the Kubernetes client
				if err := kubeClient.Create(context.TODO(), group); err != nil {
					panic(err.Error())
				}

				fmt.Println("Group created successfully!")
			}

			if addGroupName != "" {
				// add entity to group
				group := &robokubev1.Group{}
				typeName := types.NamespacedName{
					Namespace: namespace,
					Name:      addGroupName,
				}
				// get current group
				if err := kubeClient.Get(context.TODO(), typeName, group); err != nil {
					panic(err.Error())
				}
				// update group with entity, replace lastTargets with entityName
				group.Spec.LastTargets = &[]robokubev1.TargetRef{{Name: groupName, Type: robokubev1.TargetTypeGroup, Action: robokubev1.ResourceLabelAdd}}
				if err := kubeClient.Update(context.TODO(), group); err != nil {
					panic(err.Error())
				}
				fmt.Printf("group %s add to group %s\n", groupName, addGroupName)
			}

			if removeGroupName != "" {
				// remove entity from group
				group := &robokubev1.Group{}
				typeName := types.NamespacedName{
					Namespace: namespace,
					Name:      removeGroupName,
				}
				// get current group
				if err := kubeClient.Get(context.TODO(), typeName, group); err != nil {
					panic(err.Error())
				}
				// update group with entity, replace lastTargets with entityName
				group.Spec.LastTargets = &[]robokubev1.TargetRef{{Name: groupName, Type: robokubev1.TargetTypeGroup, Action: robokubev1.ResourceLabelDelete}}
				if err := kubeClient.Update(context.TODO(), group); err != nil {
					panic(err.Error())
				}
				fmt.Printf("group %s removed from group %s\n", groupName, removeGroupName)
			}

		},
	}
	groupCmd.Flags().StringP("type", "t", "Security", "Type of the group (e.g., Security)")
	groupCmd.Flags().StringP("namespace", "n", "", "Namespace of the group")
	rootCmd.AddCommand(groupCmd)

	// Task Command
	// kubectl robo task <taskName> --template <templateName> --executionTime <executionTime> --schedule <schedule> --targetGroup <targetGroup> --targetEntity <targetEntity> --permanent <permanent> --payload '{"key1": "value1", "key2": 42, "key3": ["item1", "item2"]}'
	var taskCmd = &cobra.Command{
		Use:   "task <taskName>",
		Short: "Create a new task",
		Args:  cobra.ExactArgs(1), // Requires exactly one argument: taskName
		Run: func(cmd *cobra.Command, args []string) {
			taskName := args[0]
			template, _ := cmd.Flags().GetString("template")
			executionTime, _ := cmd.Flags().GetString("executionTime")
			schedule, _ := cmd.Flags().GetString("schedule")
			permanent, _ := cmd.Flags().GetString("permanent")
			namespace, _ := cmd.Flags().GetString("namespace")
			payload, _ := cmd.Flags().GetString("payload")
			targetGroup, _ := cmd.Flags().GetString("targetGroup")
			targetEntity, _ := cmd.Flags().GetString("targetEntity")

			fmt.Println("Creating task:", taskName)
			//fmt.Println("Template:", template)
			//fmt.Println("Execution Time:", executionTime)
			//fmt.Println("Schedule:", schedule)
			//fmt.Println("permanent:", permanent)
			if namespace == "" {
				namespace = rawConfig.Contexts[currentContext].Namespace
			}

			task := &robokubev1.Task{}
			runTime := metav1.Time{}
			if executionTime != "" {
				if parsedTime, err := time.Parse(time.RFC3339, executionTime); err == nil {
					runTime = metav1.Time{Time: parsedTime}
				} else {
					panic(err.Error())
				}
			}

			if template != "" {
				// get task based on template
				typeName := types.NamespacedName{
					Namespace: namespace,
					Name:      template,
				}
				tempalteTask := &robokubev1.Task{}
				if err := kubeClient.Get(context.TODO(), typeName, tempalteTask); err != nil {
					panic(err.Error())
				}

				task.Spec = tempalteTask.Spec

				// modify task with executionTime, schedule, permanence
				if executionTime != "" {
					task.Spec.RunTime = &runTime
				}
				if schedule != "" {
					task.Spec.Schedule = schedule
				}
				if payload != "{}" {
					task.Spec.Payload = apiextensionsv1.JSON{Raw: []byte(payload)}
				}
				// permanence is boolean
				task.Spec.Persistant = permanent == "true"

				task.ObjectMeta.Name = taskName
				task.ObjectMeta.Namespace = namespace
			} else {
				// Implement logic to call kubectl robo create task ...
				task = &robokubev1.Task{ // Replace v1 with your API group version
					ObjectMeta: metav1.ObjectMeta{
						Name:      taskName,
						Namespace: namespace, // Replace with desired namespace
					},
					Spec: robokubev1.TaskSpec{ // Replace with your Spec definition
						RunTime:    &runTime,
						Schedule:   schedule,
						Persistant: permanent == "true",
						Payload:    apiextensionsv1.JSON{Raw: []byte(payload)},
						// ... other fields
					},
				}
			}

			if targetGroup != "" {
				task.Spec.Targets = &[]robokubev1.TargetRef{{Name: targetGroup, Type: robokubev1.TargetTypeGroup}}
			}
			if targetEntity != "" {
				task.Spec.Targets = &[]robokubev1.TargetRef{{Name: targetEntity, Type: robokubev1.TargetTypeEntity}}
			}

			if err := kubeClient.Create(context.TODO(), task); err != nil {
				panic(err.Error())
			}

			// Wait for task to reach a specific condition
			if schedule == "" && executionTime == "" {
				err = waitForTaskCondition(clientset, kubeClient, taskName, namespace, 2*time.Second, 300*time.Second)
				if err != nil {
					panic(err.Error())
				}
				fmt.Println("Task Completed!")
			}
		},
	}
	taskCmd.Flags().StringP("template", "t", "", "Task template")
	taskCmd.Flags().StringP("executionTime", "T", "", "Task execution time")
	taskCmd.Flags().StringP("schedule", "s", "", "Task schedule")
	taskCmd.Flags().StringP("permanent", "p", "", "Task permanence (e.g., permanent)")
	taskCmd.Flags().StringP("payload", "P", "{}", "Json payload")
	taskCmd.Flags().StringP("targetGroup", "g", "", "specify the target Group")
	taskCmd.Flags().StringP("targetEntity", "e", "", "specify the target Entity")
	rootCmd.AddCommand(taskCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // Windows
}

// Function to wait for specific conditions on the Task object
func waitForTaskCondition(clientset *kubernetes.Clientset, kubeClient client.Client, taskName, namespace string, retryInterval, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create an event watcher
	eventWatcher, err := clientset.CoreV1().Events(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.kind=Task,involvedObject.name=%s", taskName),
	})
	if err != nil {
		return fmt.Errorf("failed to create event watcher: %w", err)
	}
	defer eventWatcher.Stop()

	return wait.PollUntilContextTimeout(ctx, retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
		task := &robokubev1.Task{}
		if err := kubeClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, task); err != nil {
			return false, fmt.Errorf("failed to get task: %w", err)
		}

		// Check for new events
		select {
		case event := <-eventWatcher.ResultChan():
			if e, ok := event.Object.(*corev1.Event); ok {
				fmt.Printf("%s - %s\n", e.Reason, e.Message)
			}
		default:
			// No new events
		}

		/*
			// Check task conditions
			for _, conditionType := range targetConditions {
				conditionMet := false
				for _, condition := range task.Status.Conditions {
					if condition.Type == conditionType && condition.Status == "Success" {
						fmt.Printf("Task condition '%s': %s - %s\n", condition.Type, condition.Status, condition.Message)
						conditionMet = true
						break
					}
				}
				if !conditionMet {
					return false, nil // Keep waiting
				}
			}
		*/
		// Check task phase
		if task.Status.Phase == robokubev1.StatusCompleted || task.Status.Phase == robokubev1.StatusArchiving {
			fmt.Printf("Task phase is Completed\n")
			return true, nil
		}
		return false, nil
	})
}
