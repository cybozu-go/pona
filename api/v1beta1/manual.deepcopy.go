package v1beta1

import "encoding/json"

func (c *PodSpecApplyConfiguration) DeepCopy() *PodSpecApplyConfiguration {
	out := new(PodSpecApplyConfiguration)
	bytes, err := json.Marshal(c)
	if err != nil {
		panic("Failed to marshal")
	}
	err = json.Unmarshal(bytes, out)
	if err != nil {
		panic("Failed to unmarshal")
	}
	return out
}

func (c *DeploymentStrategyApplyConfiguration) DeepCopy() *DeploymentStrategyApplyConfiguration {
	out := new(DeploymentStrategyApplyConfiguration)
	bytes, err := json.Marshal(c)
	if err != nil {
		panic("Failed to marshal")
	}
	err = json.Unmarshal(bytes, out)
	if err != nil {
		panic("Failed to unmarshal")
	}
	return out
}
