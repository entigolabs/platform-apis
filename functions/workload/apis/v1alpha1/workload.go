package v1alpha1

// +kubebuilder:object:generate=false
type Workload interface {
	GetName() string
	GetNamespace() string
	GetWorkloadSpec() *WorkloadSpec
}

type WorkloadSpec struct {
	// +kubebuilder:validation:Enum=Always;OnFailure;Never
	// +kubebuilder:default=Always
	RestartPolicy           string          `json:"restartPolicy,omitempty"`
	ImagePullSecrets        []string        `json:"imagePullSecrets,omitempty"`
	Architecture            string          `json:"architecture,omitempty"`
	CpuRequestMultiplier    float32         `json:"-"`
	MemoryRequestMultiplier float32         `json:"-"`
	InitContainers          []InitContainer `json:"initContainers,omitempty"`
	Containers              []Container     `json:"containers"`
}

func (w *WorkloadSpec) SetCpuRequestMultiplier(multiplier float32) {
	w.CpuRequestMultiplier = multiplier
}

func (w *WorkloadSpec) SetMemoryRequestMultiplier(multiplier float32) {
	w.MemoryRequestMultiplier = multiplier
}

func (w *WorkloadSpec) SetImagePullSecrets(secrets []string) {
	w.ImagePullSecrets = secrets
}

// +kubebuilder:object:generate=false
type PodContainer interface {
	GetName() string
	GetRegistry() string
	GetRepository() string
	GetTag() string
	GetCommand() []string
	GetEnvironment() []EnvVar
	GetResources() Resources
	GetLivenessProbe() Probe
	GetReadinessProbe() Probe
	GetStartupProbe() Probe
}

type InitContainer struct {
	Name           string    `json:"name"`
	Registry       string    `json:"registry"`
	Repository     string    `json:"repository"`
	Tag            string    `json:"tag"`
	Command        []string  `json:"command,omitempty"`
	Resources      Resources `json:"resources,omitempty"`
	Environment    []EnvVar  `json:"environment,omitempty"`
	LivenessProbe  Probe     `json:"livenessProbe,omitempty"`
	ReadinessProbe Probe     `json:"readinessProbe,omitempty"`
	StartupProbe   Probe     `json:"startupProbe,omitempty"`
}

func (i *InitContainer) GetName() string {
	return i.Name
}

func (i *InitContainer) GetRegistry() string {
	return i.Registry
}

func (i *InitContainer) GetRepository() string {
	return i.Repository
}

func (i *InitContainer) GetTag() string {
	return i.Tag
}

func (i *InitContainer) GetCommand() []string {
	return i.Command
}

func (i *InitContainer) GetEnvironment() []EnvVar {
	return i.Environment
}

func (i *InitContainer) GetResources() Resources {
	return i.Resources
}

func (i *InitContainer) GetLivenessProbe() Probe {
	return i.LivenessProbe
}

func (i *InitContainer) GetReadinessProbe() Probe {
	return i.ReadinessProbe
}

func (i *InitContainer) GetStartupProbe() Probe {
	return i.StartupProbe
}

type Container struct {
	InitContainer `json:",inline"`
	ExposedPort   string    `json:"exposedPort,omitempty"`
	Services      []Service `json:"services,omitempty"`
}

type Resources struct {
	Limits Limits `json:"limits"`
}

type Limits struct {
	CPU float32 `json:"cpu,omitempty"`
	RAM float32 `json:"ram,omitempty"`
}

type Service struct {
	Name string `json:"name"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:Enum=TCP;SCTP;UDP;GRPC
	// +kubebuilder:default=TCP
	Protocol string `json:"protocol,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// +kubebuilder:default=false
	Secret bool `json:"secret,omitempty"`
}

type Probe struct {
	Path string `json:"path"`
	Port string `json:"port"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	SuccessThreshold int32 `json:"successThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=30
	TerminationGracePeriodSeconds int32 `json:"terminationGracePeriodSeconds,omitempty"`
}
