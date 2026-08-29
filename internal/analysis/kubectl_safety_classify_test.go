package analysis

import "testing"

func TestClassifyKubectlCommand_ReadOnlyVerbs(t *testing.T) {
	cases := []string{
		"kubectl get pods",
		"kubectl describe pod x",
		"kubectl logs x",
		"kubectl top pods",
		"kubectl explain pod",
		"kubectl version",
	}
	for _, c := range cases {
		risk, _ := ClassifyKubectlCommand(c)
		if risk != RiskReadOnly {
			t.Errorf("%q should be RiskReadOnly; got %v", c, risk)
		}
	}
}

func TestClassifyKubectlCommand_MutatingVerbs(t *testing.T) {
	cases := []string{
		"kubectl scale deploy/web --replicas=3",
		"kubectl rollout restart deploy/web",
		"kubectl label pod x foo=bar",
		"kubectl patch deploy/web -p '{}'",
		"kubectl apply -f file.yaml",
	}
	for _, c := range cases {
		risk, _ := ClassifyKubectlCommand(c)
		if risk != RiskMutating {
			t.Errorf("%q should be RiskMutating; got %v", c, risk)
		}
	}
}

func TestClassifyKubectlCommand_DestructiveVerbs(t *testing.T) {
	cases := []string{
		"kubectl delete pod x",
		"kubectl drain node1",
		"kubectl cordon node1",
		"kubectl uncordon node1",
	}
	for _, c := range cases {
		risk, _ := ClassifyKubectlCommand(c)
		if risk != RiskDestructive {
			t.Errorf("%q should be RiskDestructive; got %v", c, risk)
		}
	}
}

func TestClassifyKubectlCommand_UnknownVerb(t *testing.T) {
	risk, _ := ClassifyKubectlCommand("kubectl mystery thing")
	if risk != RiskUnknown {
		t.Errorf("unknown verb should be RiskUnknown; got %v", risk)
	}
}

func TestClassifyKubectlCommand_TooFewTokens(t *testing.T) {
	risk, _ := ClassifyKubectlCommand("kubectl")
	if risk != RiskUnknown {
		t.Errorf("too-few-tokens should be RiskUnknown; got %v", risk)
	}
}

func TestClassifyKubectlCommand_BadShlex(t *testing.T) {
	risk, _ := ClassifyKubectlCommand("kubectl 'unclosed")
	if risk != RiskUnknown {
		t.Errorf("malformed shell quoting should be RiskUnknown; got %v", risk)
	}
}
