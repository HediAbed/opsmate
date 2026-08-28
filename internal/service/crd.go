package service

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// CRDError adds operation and resource context to a CRD failure.
type CRDError struct {
	Operation string
	Resource  string
	Err       error
}

func (e *CRDError) Error() string {
	detail := "unknown error"
	if e.Err != nil {
		detail = e.Err.Error()
	}
	if e.Resource != "" {
		return fmt.Sprintf("crd %s %s: %s", e.Operation, e.Resource, detail)
	}
	return fmt.Sprintf("crd %s: %s", e.Operation, detail)
}

func (e *CRDError) Unwrap() error { return e.Err }

// CRD contains the fields displayed for a custom resource definition.
type CRD struct {
	Name     string
	Group    string
	Plural   string
	Singular string
	Kind     string
	Scope    string
	Versions []string
	Resource string
	Age      string
}

type CRDsMsg struct {
	CRDs []CRD
	Err  error
}

type rawCRDItem struct {
	Metadata struct {
		Name              string    `json:"name"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Group string `json:"group"`
		Names struct {
			Kind     string `json:"kind"`
			Plural   string `json:"plural"`
			Singular string `json:"singular"`
		} `json:"names"`
		Scope    string          `json:"scope"`
		Versions []rawCRDVersion `json:"versions"`
	} `json:"spec"`
}

type crdList struct {
	Items []rawCRDItem `json:"items"`
}

type rawCRDVersion struct {
	Name    string `json:"name"`
	Served  bool   `json:"served"`
	Storage bool   `json:"storage"`
}

func FetchCRDs() tea.Cmd {
	return func() tea.Msg {
		payload, err := runKubectlJSON[crdList](KubectlReadTimeout, "get", "crd", "-o", "json")
		if err != nil {
			return CRDsMsg{Err: &CRDError{Operation: "list", Err: err}}
		}
		return CRDsMsg{CRDs: projectListItems(payload.Items, crdFromRaw)}
	}
}

func crdFromRaw(item rawCRDItem) CRD {
	served := servedVersionNames(item.Spec.Versions)
	return CRD{
		Name:     item.Metadata.Name,
		Group:    item.Spec.Group,
		Plural:   item.Spec.Names.Plural,
		Singular: item.Spec.Names.Singular,
		Kind:     item.Spec.Names.Kind,
		Scope:    item.Spec.Scope,
		Versions: served,
		Resource: crdResourceArg(item.Spec.Names.Plural, item.Spec.Group),
		Age:      formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

func servedVersionNames(versions []rawCRDVersion) []string {
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		if v.Served {
			out = append(out, v.Name)
		}
	}
	return out
}

func crdResourceArg(plural, group string) string {
	if plural == "" || group == "" {
		return ""
	}
	return plural + "." + group
}

// CRDInstance contains the metadata common to resources with arbitrary schemas.
type CRDInstance struct {
	Name      string
	Namespace string
	Age       string
}

type CRDInstancesMsg struct {
	Resource  string
	Namespace string
	Instances []CRDInstance
	Err       error
}

type rawCRDInstance struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
}

// ListCRDInstances fetches instances in one namespace or across the cluster.
func ListCRDInstances(resource, namespace string) tea.Cmd {
	return func() tea.Msg {
		items, err := listKubectlItems[rawCRDInstance](resource, namespace)
		if err != nil {
			return CRDInstancesMsg{
				Resource:  resource,
				Namespace: namespace,
				Err:       &CRDError{Operation: "list-instances", Resource: resource, Err: err},
			}
		}
		return CRDInstancesMsg{
			Resource:  resource,
			Namespace: namespace,
			Instances: projectListItems(items, crdInstanceFromRaw),
		}
	}
}

func crdInstanceFromRaw(item rawCRDInstance) CRDInstance {
	return CRDInstance{
		Name:      item.Metadata.Name,
		Namespace: item.Metadata.Namespace,
		Age:       formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

func JoinVersions(versions []string) string {
	return strings.Join(versions, ",")
}
