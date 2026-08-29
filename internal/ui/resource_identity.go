package ui

type resourceIdentity struct {
	Kind      string
	Namespace string
	Name      string
}

func (identity resourceIdentity) key() string {
	return identity.Kind + "\x00" + identity.Namespace + "\x00" + identity.Name
}

func namespacedResourceKey(namespace, name string) string {
	return namespace + "\x00" + name
}

func namespacesMatch(first, second string) bool {
	return first == second
}

func displayResourceName(namespace, name string, qualify bool) string {
	if qualify && namespace != "" {
		return namespace + "/" + name
	}
	return name
}
