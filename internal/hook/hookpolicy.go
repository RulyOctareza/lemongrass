package hook

import (
	"time"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/guard"
)

const policyFetchTimeout = 500 * time.Millisecond

// hookPolicy is the launch project's own policy, else the global one. A vault that is down or serving a policy that fails validation leaves only the built-in catalog in force.
func hookPolicy(projectID string) guard.Policy {
	return hookPolicyFrom(gatekeeper.SocketPath(), projectID)
}

func hookPolicyFrom(socketPath, projectID string) guard.Policy {
	client := &gatekeeper.Client{SocketPath: socketPath, Timeout: policyFetchTimeout}
	policy, err := client.CurrentPolicy(projectID)
	if err != nil {
		return guard.Policy{}
	}
	return policy
}
