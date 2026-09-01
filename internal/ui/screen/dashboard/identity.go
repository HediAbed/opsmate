package dashboard

const podResourceType = "pods"

type podIdentity struct {
	Namespace string
	Name      string
}

func namespacedResourceKey(namespace, name string) string {
	return namespace + "\x00" + name
}

func displayPodName(namespace, name string, qualify bool) string {
	if qualify && namespace != "" {
		return namespace + "/" + name
	}
	return name
}
