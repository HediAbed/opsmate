package cluster

import "strings"

type CRD struct {
	Name             string
	Group            string
	Plural           string
	Singular         string
	Kind             string
	Scope            string
	Versions         []string
	PreferredVersion string
	Resource         string
	Age              string
}

type CRDsMsg struct {
	CRDs []CRD
	Err  error
}

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

func JoinVersions(versions []string) string {
	return strings.Join(versions, ",")
}
