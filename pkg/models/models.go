package models

// Contact defines a person or group to be notified
type Contact struct {
	ID    string `yaml:"id" json:"id"`
	Email string `yaml:"email" json:"email"`
}

// HostGroup defines a logical cluster of hosts
type HostGroup struct {
	ID    string `yaml:"id" json:"id"`
	Alias string `yaml:"alias" json:"alias"`
}

// ServiceGroup defines a logical cluster of services
type ServiceGroup struct {
	ID    string `yaml:"id" json:"id"`
	Alias string `yaml:"alias" json:"alias"`
}

// Command defines the raw executable command line
type Command struct {
	ID          string `yaml:"id" json:"id"`
	CommandLine string `yaml:"command_line" json:"command_line"`
}

// Host represents a monitored machine with template support
type Host struct {
	ID           string            `yaml:"id" json:"id"`
	Use          string            `yaml:"use" json:"use"`           
	Address      string            `yaml:"address" json:"address"`
	CheckCommand string            `yaml:"check_command" json:"check_command"`
	Alias        string            `yaml:"alias" json:"alias"`
	Contacts     []string          `yaml:"contacts" json:"contacts"`
	HostGroups   []string          `yaml:"hostgroups" json:"hostgroups"`
	Macros       map[string]string `yaml:"macros" json:"macros"`
	Register     *bool             `yaml:"register" json:"register"`
}

// Service represents a specific check with template support
type Service struct {
	ID             string            `yaml:"id" json:"id"`
	Use            string            `yaml:"use" json:"use"`
	HostName       string            `yaml:"host_name" json:"host_name"`
	CheckCommand   string            `yaml:"check_command" json:"check_command"`
	NormalInterval int               `yaml:"normal_interval" json:"normal_interval"`
	RetryInterval  int               `yaml:"retry_interval" json:"retry_interval"`
	MaxAttempts    int               `yaml:"max_attempts" json:"max_attempts"`
	Contacts       []string          `yaml:"contacts" json:"contacts"`
	ServiceGroups  []string          `yaml:"servicegroups" json:"servicegroups"`
	Macros         map[string]string `yaml:"macros" json:"macros"`
	Register       *bool             `yaml:"register" json:"register"`
}

// GlobalConfig is the final payload sent to the Scheduler
type GlobalConfig struct {
	Commands      []Command      `yaml:"commands" json:"commands"`
	Contacts      []Contact      `yaml:"contacts" json:"contacts"`
	HostGroups    []HostGroup    `yaml:"hostgroups" json:"hostgroups"`
	ServiceGroups []ServiceGroup `yaml:"servicegroups" json:"servicegroups"`
	Hosts         []Host         `yaml:"hosts" json:"hosts"`
	Services      []Service      `yaml:"services" json:"services"`
}
