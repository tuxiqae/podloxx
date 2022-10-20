package kubernetes

import (
	"context"
	"os"

	"github.com/mogenius/mo-go/logger"

	apiv1 "k8s.io/api/core/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyconfapp "k8s.io/client-go/applyconfigurations/apps/v1"
	applyconfcore "k8s.io/client-go/applyconfigurations/core/v1"
	applyconfmeta "k8s.io/client-go/applyconfigurations/meta/v1"
)

func Deploy() {
	provider, err := NewKubeProviderLocal()
	if err != nil {
		panic(err)
	}

	addRbac(provider)
	addRedis(provider)
	addDaemonSet(provider)
}

func addRbac(kubeProvider *KubeProvider) error {
	serviceAccount := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: SERVICEACCOUNTNAME,
		},
	}
	clusterRole := &rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: CLUSTERROLENAME,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{"", "extensions", "apps"},
				Resources: RBACRESOURCES,
				Verbs:     []string{"list", "get", "watch"},
			},
		},
	}
	clusterRoleBinding := &rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: CLUSTERROLEBINDINGNAME,
		},
		RoleRef: rbac.RoleRef{
			Name:     CLUSTERROLENAME,
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      SERVICEACCOUNTNAME,
				Namespace: NAMESPACE,
			},
		},
	}

	// CREATE RBAC
	logger.Log.Info("Creating podloxx RBAC ...")
	_, err := kubeProvider.ClientSet.CoreV1().ServiceAccounts(NAMESPACE).Create(context.TODO(), serviceAccount, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	_, err = kubeProvider.ClientSet.RbacV1().ClusterRoles().Create(context.TODO(), clusterRole, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	_, err = kubeProvider.ClientSet.RbacV1().ClusterRoleBindings().Create(context.TODO(), clusterRoleBinding, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	logger.Log.Info("Created podloxx RBAC.")
	return nil
}

func addDaemonSet(kubeProvider *KubeProvider) {
	daemonSetClient := kubeProvider.ClientSet.AppsV1().DaemonSets(apiv1.NamespaceDefault)

	daemonsetContainer := applyconfcore.Container()
	daemonsetContainer.WithName(DAEMONSETNAME)
	daemonsetContainer.WithImage(DAEMONSETIMAGE)
	daemonsetContainer.WithImagePullPolicy(core.PullAlways)
	daemonsetContainer.WithEnv(
		applyconfcore.EnvVar().WithName("STAGE").WithValue(os.Getenv("STAGE")),
		applyconfcore.EnvVar().WithName("INTERFACE_PREFIX").WithValue(os.Getenv("INTERFACE_PREFIX")),
		applyconfcore.EnvVar().WithName("OWN_NODE_NAME").WithValueFrom(
			applyconfcore.EnvVarSource().WithFieldRef(
				applyconfcore.ObjectFieldSelector().WithAPIVersion("v1").WithFieldPath("spec.nodeName"),
			),
		),
		applyconfcore.EnvVar().WithName("OWN_POD_NAME").WithValueFrom(
			applyconfcore.EnvVarSource().WithFieldRef(
				applyconfcore.ObjectFieldSelector().WithAPIVersion("v1").WithFieldPath("metadata.name"),
			),
		),
		applyconfcore.EnvVar().WithName("OWN_NAMESPACE").WithValueFrom(
			applyconfcore.EnvVarSource().WithFieldRef(
				applyconfcore.ObjectFieldSelector().WithAPIVersion("v1").WithFieldPath("metadata.namespace"),
			),
		),
	)

	caps := applyconfcore.Capabilities().WithDrop("ALL")

	caps = caps.WithAdd("NET_RAW")
	caps = caps.WithAdd("NET_ADMIN")
	caps = caps.WithAdd("SYS_ADMIN")
	caps = caps.WithAdd("SYS_PTRACE")
	caps = caps.WithAdd("DAC_OVERRIDE")
	caps = caps.WithAdd("SYS_RESOURCE")
	daemonsetContainer.WithSecurityContext(applyconfcore.SecurityContext().WithCapabilities(caps))

	agentResourceLimits := core.ResourceList{
		"cpu":               resource.MustParse("1000m"),
		"memory":            resource.MustParse("512Mi"),
		"ephemeral-storage": resource.MustParse("100Mi"),
	}
	agentResourceRequests := core.ResourceList{
		"cpu":               resource.MustParse("500m"),
		"memory":            resource.MustParse("128Mi"),
		"ephemeral-storage": resource.MustParse("10Mi"),
	}
	agentResources := applyconfcore.ResourceRequirements().WithRequests(agentResourceRequests).WithLimits(agentResourceLimits)
	daemonsetContainer.WithResources(agentResources)

	// Host procfs is needed inside the container because we need access to
	//	the network namespaces of processes on the machine.
	//
	procfsVolume := applyconfcore.Volume()
	procfsVolume.WithName(PROCFSVOLUMENAME).WithHostPath(applyconfcore.HostPathVolumeSource().WithPath("/proc"))
	procfsVolumeMount := applyconfcore.VolumeMount().WithName(PROCFSVOLUMENAME).WithMountPath(PROCFSMOUNTPATH).WithReadOnly(true)
	daemonsetContainer.WithVolumeMounts(procfsVolumeMount)

	// We need access to /sys in order to install certain eBPF tracepoints
	//
	sysfsVolume := applyconfcore.Volume()
	sysfsVolume.WithName(SYSFSVOLUMENAME).WithHostPath(applyconfcore.HostPathVolumeSource().WithPath("/sys"))
	sysfsVolumeMount := applyconfcore.VolumeMount().WithName(SYSFSVOLUMENAME).WithMountPath(SYSFSMOUNTPATH).WithReadOnly(true)
	daemonsetContainer.WithVolumeMounts(sysfsVolumeMount)

	podSpec := applyconfcore.PodSpec()
	podSpec.WithHostNetwork(true)
	podSpec.WithDNSPolicy(core.DNSClusterFirstWithHostNet)
	podSpec.WithTerminationGracePeriodSeconds(0)
	podSpec.WithServiceAccountName(SERVICEACCOUNTNAME)

	podSpec.WithContainers(daemonsetContainer)
	podSpec.WithVolumes(procfsVolume, sysfsVolume)

	applyOptions := metav1.ApplyOptions{
		Force:        true,
		FieldManager: DAEMONSETNAME,
	}

	labelSelector := applyconfmeta.LabelSelector()
	labelSelector.WithMatchLabels(map[string]string{"app": DAEMONSETNAME})

	podTemplate := applyconfcore.PodTemplateSpec()
	podTemplate.WithLabels(map[string]string{
		"app": DAEMONSETNAME,
	})
	podTemplate.WithSpec(podSpec)

	daemonSet := applyconfapp.DaemonSet(DAEMONSETNAME, NAMESPACE)
	daemonSet.WithSpec(applyconfapp.DaemonSetSpec().WithSelector(labelSelector).WithTemplate(podTemplate))

	// Create DaemonSet
	logger.Log.Info("Creating podloxx daemonset ...")
	result, err := daemonSetClient.Apply(context.TODO(), daemonSet, applyOptions)
	if err != nil {
		panic(err)
	}
	logger.Log.Info("Created podloxx daemonset.", result.GetObjectMeta().GetName(), ".")
}

func addRedis(kubeProvider *KubeProvider) {
	deploymentClient := kubeProvider.ClientSet.AppsV1().Deployments(apiv1.NamespaceDefault)

	deploymentContainer := applyconfcore.Container()
	deploymentContainer.WithName(REDISNAME)
	deploymentContainer.WithImage(REDISIMAGE)
	deploymentContainer.WithEnv(
		applyconfcore.EnvVar().WithName("STAGE").WithValue(os.Getenv("STAGE")),
	)
	agentResourceLimits := core.ResourceList{
		"cpu":               resource.MustParse("300m"),
		"memory":            resource.MustParse("256Mi"),
		"ephemeral-storage": resource.MustParse("100Mi"),
	}
	agentResourceRequests := core.ResourceList{
		"cpu":               resource.MustParse("100m"),
		"memory":            resource.MustParse("128Mi"),
		"ephemeral-storage": resource.MustParse("10Mi"),
	}
	agentResources := applyconfcore.ResourceRequirements().WithRequests(agentResourceRequests).WithLimits(agentResourceLimits)
	deploymentContainer.WithResources(agentResources)

	podSpec := applyconfcore.PodSpec()
	podSpec.WithTerminationGracePeriodSeconds(0)
	podSpec.WithServiceAccountName(SERVICEACCOUNTNAME)

	podSpec.WithContainers(deploymentContainer)

	applyOptions := metav1.ApplyOptions{
		Force:        true,
		FieldManager: REDISNAME,
	}

	labelSelector := applyconfmeta.LabelSelector()
	labelSelector.WithMatchLabels(map[string]string{"app": REDISNAME})

	podTemplate := applyconfcore.PodTemplateSpec()
	podTemplate.WithLabels(map[string]string{
		"app": REDISNAME,
	})
	podTemplate.WithSpec(podSpec)

	deployment := applyconfapp.Deployment(REDISNAME, NAMESPACE)
	deployment.WithSpec(applyconfapp.DeploymentSpec().WithSelector(labelSelector).WithTemplate(podTemplate))

	// Create Redis Deployment
	logger.Log.Info("Creating podloxx redis ...")
	result, err := deploymentClient.Apply(context.TODO(), deployment, applyOptions)
	if err != nil {
		panic(err)
	}
	logger.Log.Info("Created podloxx redis.", result.GetObjectMeta().GetName(), ".")
}
