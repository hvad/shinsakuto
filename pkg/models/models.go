package models

// Contact defines a person or group to be notified
type Contact struct {
	ID    string `yaml:"id" json:"id"`
	Email string `yaml:"email" json:"email"`
}

// Command defines the raw command line to be executed
type Command struct {
	ID          string `yaml:"id" json:"id"`
	CommandLine string `yaml:"command_line" json:"command_line"`
}

// Host represents a monitored machine
type Host struct {
	ID           string   `yaml:"id" json:"id"`
	Use          string   `yaml:"use" json:"use"`           
	Address      string   `yaml:"address" json:"address"`
	CheckCommand string   `yaml:"check_command" json:"check_command"`
	Alias        string   `yaml:"alias" json:"alias"`
	Contacts     []string `yaml:"contacts" json:"contacts"` 
	Register     *bool    `yaml:"register" json:"register"` 
}

// Service represents a specific check bound to a host
type Service struct {
	ID             string   `yaml:"id" json:"id"`
	Use            string   `yaml:"use" json:"use"`
	HostName       string   `yaml:"host_name" json:"host_name"`
	CheckCommand   string   `yaml:"check_command" json:"check_command"`
	NormalInterval int      `yaml:"normal_interval" json:"normal_interval"`
	RetryInterval  int      `yaml:"retry_interval" json:"retry_interval"`
	MaxAttempts    int      `yaml:"max_attempts" json:"max_attempts"`
	Contacts       []string `yaml:"contacts" json:"contacts"` 
	Register       *bool    `yaml:"register" json:"register"`
}

// GlobalConfig is the container for exporting to the Scheduler
type GlobalConfig struct {
	Commands []Command `yaml:"commands" json:"commands"`
	Contacts []Contact `yaml:"contacts" json:"contacts"`
	Hosts    []Host    `yaml:"hosts" json:"hosts"`
	Services []Service `yaml:"services" json:"services"`
}
