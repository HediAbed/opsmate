package cluster

import (
	"maps"
	"strings"
	"time"

	model "github.com/HediAbed/opsmate/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

func projectIngresses(items []networkingv1.Ingress, now time.Time) []model.Ingress {
	return projectSlice(items, now, projectIngress)
}

func projectIngress(item networkingv1.Ingress, now time.Time) model.Ingress {
	className := ""
	if item.Spec.IngressClassName != nil {
		className = *item.Spec.IngressClassName
	}
	return model.Ingress{
		Name:      item.Name,
		Namespace: item.Namespace,
		Class:     className,
		Hosts:     projectedIngressHosts(item.Spec.Rules),
		Address:   projectedIngressAddresses(item.Status.LoadBalancer.Ingress),
		Ports:     projectedIngressPorts(item.Spec.TLS),
		Age:       projectedAge(now, item.CreationTimestamp.Time),
	}
}

func projectedIngressHosts(rules []networkingv1.IngressRule) string {
	hosts := make([]string, 0, len(rules))
	for _, rule := range rules {
		if rule.Host != "" {
			hosts = appendUnique(hosts, rule.Host)
		}
	}
	return strings.Join(hosts, ",")
}

func projectedIngressAddresses(ingresses []networkingv1.IngressLoadBalancerIngress) string {
	addresses := make([]string, 0, len(ingresses))
	for _, ingress := range ingresses {
		switch {
		case ingress.Hostname != "":
			addresses = appendUnique(addresses, ingress.Hostname)
		case ingress.IP != "":
			addresses = appendUnique(addresses, ingress.IP)
		}
	}
	return strings.Join(addresses, ",")
}

func projectedIngressPorts(tls []networkingv1.IngressTLS) string {
	if len(tls) > 0 {
		return ingressHTTPSPorts
	}
	return ingressHTTPPorts
}

func projectNetworkPolicies(items []networkingv1.NetworkPolicy, now time.Time) []model.NetworkPolicy {
	return projectSlice(items, now, func(item networkingv1.NetworkPolicy, now time.Time) model.NetworkPolicy {
		policyTypes := make([]string, 0, len(item.Spec.PolicyTypes))
		for _, policyType := range item.Spec.PolicyTypes {
			policyTypes = append(policyTypes, string(policyType))
		}
		return model.NetworkPolicy{
			Name:        item.Name,
			Namespace:   item.Namespace,
			PodSelector: maps.Clone(item.Spec.PodSelector.MatchLabels),
			PolicyTypes: policyTypes,
			Age:         projectedAge(now, item.CreationTimestamp.Time),
		}
	})
}

func projectPVCs(items []corev1.PersistentVolumeClaim, now time.Time) []model.PersistentVolumeClaim {
	return projectSlice(items, now, func(item corev1.PersistentVolumeClaim, now time.Time) model.PersistentVolumeClaim {
		accessModes := make([]string, 0, len(item.Spec.AccessModes))
		for _, accessMode := range item.Spec.AccessModes {
			accessModes = append(accessModes, string(accessMode))
		}
		storageClass := ""
		if item.Spec.StorageClassName != nil {
			storageClass = *item.Spec.StorageClassName
		}
		return model.PersistentVolumeClaim{
			Name:         item.Name,
			Namespace:    item.Namespace,
			Status:       string(item.Status.Phase),
			Volume:       item.Spec.VolumeName,
			Capacity:     projectedPVCCapacity(item),
			AccessModes:  accessModes,
			StorageClass: storageClass,
			Age:          projectedAge(now, item.CreationTimestamp.Time),
		}
	})
}

func projectedPVCCapacity(item corev1.PersistentVolumeClaim) string {
	if quantity, found := item.Status.Capacity[corev1.ResourceStorage]; found && !quantity.IsZero() {
		return quantity.String()
	}
	if quantity, found := item.Spec.Resources.Requests[corev1.ResourceStorage]; found {
		return quantity.String()
	}
	return ""
}
