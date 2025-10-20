package apis

import "errors"

type Environment struct {
	CPURequestMultiplier    float32  `json:"cpuRequestMultiplier"`
	MemoryRequestMultiplier float32  `json:"memoryRequestMultiplier"`
	ImagePullSecrets        []string `json:"imagePullSecrets,omitempty"`
}

func (e Environment) Validate() error {
	if e.CPURequestMultiplier <= 0 {
		return errors.New("cpuRequestMultiplier must be greater than 0")
	}
	if e.MemoryRequestMultiplier <= 0 {
		return errors.New("memoryRequestMultiplier must be greater than 0")
	}
	return nil
}
